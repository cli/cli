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
				fmt.Fprintf(stderr, "%s %s: %s\n", utils.Red("X"), hostname, err)
				fmt.Fprintln(stderr,
					"The token in GITHUB_TOKEN is valid but missing scopes that gh requires to function.")
			} else {
				fmt.Fprintf(stderr, "%s %s: authentication failed\n", utils.Red("X"), hostname)
				fmt.Fprintln(stderr)
				fmt.Fprintf(stderr,
					"The token in GITHUB_TOKEN is invalid.\n")
			}
			fmt.Fprintf(stderr,
				"Please visit https://%s/settings/tokens and create a new token with 'repo', 'read:org', and 'gist' scopes.\n", hostname)
			return cmdutil.SilentError
		} else {
			username, err := api.CurrentLoginName(apiClient, hostname)
			if err != nil {
				return fmt.Errorf("%s %s: api call failed: %s\n", utils.Red("X"), hostname, err)
			}
			fmt.Fprintf(stderr,
				"%s token valid for %s as %s\n", utils.GreenCheck(), hostname, utils.Bold(username))
			proto, _ := cfg.Get(hostname, "git_protocol")
			if proto != "" {
				fmt.Fprintln(stderr)
				fmt.Fprintf(stderr,
					"Git operations for %s configured to use %s protocol.\n", hostname, utils.Bold(proto))
			}
		}

		return nil
	}

	statusInfo := map[string][]string{}

	hostnames, err := cfg.Hosts()
	if len(hostnames) == 0 || err != nil {
		fmt.Fprintf(stderr,
			"You are not logged into any GitHub hosts. Run %s to authenticate.\n", utils.Bold("gh auth login"))
		return cmdutil.SilentError
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

		statusInfo[hostname] = []string{}
		addMsg := func(x string, ys ...interface{}) {
			statusInfo[hostname] = append(statusInfo[hostname], fmt.Sprintf(x, ys...))
		}

		_, err = apiClient.HasMinimumScopes(hostname)
		if err != nil {
			var missingScopes *api.MissingScopesError
			if errors.As(err, &missingScopes) {
				addMsg("%s %s: %s\n", utils.Red("X"), hostname, err)
				addMsg("- To enable the missing scopes, please run %s %s\n",
					utils.Bold("gh auth refresh -h"),
					utils.Bold(hostname))
			} else {
				addMsg("%s %s: authentication failed\n", utils.Red("X"), hostname)
				addMsg("- The configured token for %s is no longer valid.", utils.Bold(hostname))
				addMsg("- To re-authenticate, please run %s %s",
					utils.Bold("gh auth login -h"), utils.Bold(hostname))
				addMsg("- To forget about this host, please run %s %s",
					utils.Bold("gh auth logout -h"), utils.Bold(hostname))
			}
			failed = true
		} else {
			username, err := api.CurrentLoginName(apiClient, hostname)
			if err != nil {
				addMsg("%s %s: api call failed: %s\n", utils.Red("X"), hostname, err)
			}
			addMsg("%s Logged in to %s as %s", utils.GreenCheck(), hostname, utils.Bold(username))
			proto, _ := cfg.Get(hostname, "git_protocol")
			if proto != "" {
				addMsg("Git operations for %s configured to use %s protocol.", hostname, utils.Bold(proto))
			}
		}

		// NB we could take this opportunity to add or fix the "user" key in the hosts config. I chose
		// not to since I wanted this command to be read-only.
	}

	for hostname, lines := range statusInfo {
		fmt.Fprintf(stderr, "%s\n", utils.Bold(hostname))
		for _, line := range lines {
			fmt.Fprintf(stderr, "\t%s\n", line)
		}
	}

	if failed {
		return cmdutil.SilentError
	}

	return nil
}
