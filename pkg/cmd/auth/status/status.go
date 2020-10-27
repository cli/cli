package status

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

type StatusOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	Config     func() (config.Config, error)

	Hostname  string
	ShowToken bool
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
			if runF != nil {
				return runF(opts)
			}

			return statusRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Hostname, "hostname", "h", "", "Check a specific hostname's auth status")
	cmd.Flags().BoolVarP(&opts.ShowToken, "show-token", "t", false, "Display the auth token")

	return cmd
}

func statusRun(opts *StatusOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	// TODO check tty

	stderr := opts.IO.ErrOut

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

		token, tokenSource, _ := cfg.GetWithSource(hostname, "oauth_token")
		tokenIsWriteable := cfg.CheckWriteable(hostname, "oauth_token") == nil

		statusInfo[hostname] = []string{}
		addMsg := func(x string, ys ...interface{}) {
			statusInfo[hostname] = append(statusInfo[hostname], fmt.Sprintf(x, ys...))
		}

		err = apiClient.HasMinimumScopes(hostname)
		if err != nil {
			var missingScopes *api.MissingScopesError
			if errors.As(err, &missingScopes) {
				addMsg("%s %s: the token in %s is %s", utils.Red("X"), hostname, tokenSource, err)
				if tokenIsWriteable {
					addMsg("- To request missing scopes, run: %s %s\n",
						utils.Bold("gh auth refresh -h"),
						utils.Bold(hostname))
				}
			} else {
				addMsg("%s %s: authentication failed", utils.Red("X"), hostname)
				addMsg("- The %s token in %s is no longer valid.", utils.Bold(hostname), tokenSource)
				if tokenIsWriteable {
					addMsg("- To re-authenticate, run: %s %s",
						utils.Bold("gh auth login -h"), utils.Bold(hostname))
					addMsg("- To forget about this host, run: %s %s",
						utils.Bold("gh auth logout -h"), utils.Bold(hostname))
				}
			}
			failed = true
		} else {
			username, err := api.CurrentLoginName(apiClient, hostname)
			if err != nil {
				addMsg("%s %s: api call failed: %s", utils.Red("X"), hostname, err)
			}
			addMsg("%s Logged in to %s as %s (%s)", utils.GreenCheck(), hostname, utils.Bold(username), tokenSource)
			proto, _ := cfg.Get(hostname, "git_protocol")
			if proto != "" {
				addMsg("%s Git operations for %s configured to use %s protocol.",
					utils.GreenCheck(), hostname, utils.Bold(proto))
			}
			tokenDisplay := "*******************"
			if opts.ShowToken {
				tokenDisplay = token
			}
			addMsg("%s Token: %s", utils.GreenCheck(), tokenDisplay)
		}
		addMsg("")

		// NB we could take this opportunity to add or fix the "user" key in the hosts config. I chose
		// not to since I wanted this command to be read-only.
	}

	for _, hostname := range hostnames {
		lines, ok := statusInfo[hostname]
		if !ok {
			continue
		}
		fmt.Fprintf(stderr, "%s\n", utils.Bold(hostname))
		for _, line := range lines {
			fmt.Fprintf(stderr, "  %s\n", line)
		}
	}

	if failed {
		return cmdutil.SilentError
	}

	return nil
}
