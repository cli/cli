package status

import (
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/internal/ghinstance"
	"github.com/cli/cli/pkg/cmd/auth/client"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

type StatusOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	Config     func() (config.Config, error)
	Token      string
	Hostname   string
}

func NewCmdStatus(f *cmdutil.Factory, runF func(*StatusOptions) error) *cobra.Command {
	opts := &StatusOptions{
		HttpClient: f.HttpClient,
		IO:         f.IOStreams,
		Config:     f.Config,
	}

	cmd := &cobra.Command{
		Use:   "status",
		Args:  cobra.ExactArgs(0),
		Short: "View authentication status",
		Long: heredoc.Doc(`Verifies and displays information about your authentication state.
			
			This command will test your authentication state for each GitHub host that gh knows about and
			report on any issues.
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO support other names
			opts.Token = os.Getenv("GITHUB_TOKEN")

			if opts.Token != "" && opts.Hostname == "" {
				opts.Hostname = ghinstance.Default()
			}

			if runF != nil {
				return runF(opts)
			}

			return statusRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Hostname, "hostname", "h", "", "Check a specific hostname's auth status")

	return cmd
}

func statusRun(opts *StatusOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	// TODO check tty

	stderr := opts.IO.ErrOut

	if opts.Token != "" {
		hostname := opts.Hostname
		err := cfg.Set(opts.Hostname, "oauth_token", opts.Token)
		if err != nil {
			return err
		}

		apiClient, err := client.ClientFromCfg(hostname, cfg)
		if err != nil {
			return err
		}

		_, err = apiClient.HasMinimumScopes(hostname)
		if err != nil {
			var missingScopes *api.MissingScopesError
			if errors.As(err, &missingScopes) {
				return fmt.Errorf("%s %s: %s", utils.Red("X"), hostname, err)
			} else {
				return fmt.Errorf("%s %s: authentication failed", utils.Red("X"), hostname)
			}
		} else {
			username, err := api.CurrentLoginName(apiClient, hostname)
			if err != nil {
				return fmt.Errorf("%s %s: api call failed: %s\n", utils.Red("X"), hostname, err)
			}
			fmt.Fprintf(stderr, "%s token valid for %s as %s\n", utils.GreenCheck(), hostname, utils.Bold(username))
		}

		return nil
	}

	hostnames, err := cfg.Hosts()
	if len(hostnames) == 0 || err != nil {
		fmt.Fprintf(stderr, "You are not logged into any GitHub hosts. Run 'gh auth login' to authenticate.\n")
		return nil
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	apiClient := api.NewClientFromHTTP(httpClient)

	var failed bool

	for _, hostname := range hostnames {
		if opts.Hostname != "" && opts.Hostname != hostname {
			continue
		}
		_, err = apiClient.HasMinimumScopes(hostname)
		if err != nil {
			var missingScopes *api.MissingScopesError
			if errors.As(err, &missingScopes) {
				fmt.Fprintf(stderr, "%s %s: %s\n", utils.Red("X"), hostname, err)
			} else {
				fmt.Fprintf(stderr, "%s %s: authentication failed\n", utils.Red("X"), hostname)
			}
			failed = true
		} else {
			username, err := api.CurrentLoginName(apiClient, hostname)
			if err != nil {
				fmt.Fprintf(stderr, "%s %s: api call failed: %s\n", utils.Red("X"), hostname, err)
			}
			fmt.Fprintf(stderr, "%s Logged in to %s as %s\n", utils.GreenCheck(), hostname, utils.Bold(username))
		}

		// NB we could take this opportunity to add or fix the "user" key in the hosts config. I chose
		// not to since I wanted this command to be read-only.
	}

	if failed {
		// TODO unsure about this; want non-zero exit but don't need to print anything more. Is the
		// non-zero exit worth it? Should we tweak error handling to not print "" errors?
		return errors.New("")
	}

	return nil
}
