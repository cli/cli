package status

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/cmd/auth/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
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
	authCfg := cfg.Authentication()

	// TODO check tty

	stderr := opts.IO.ErrOut

	cs := opts.IO.ColorScheme()

	statusInfo := map[string][]string{}

	hostnames := authCfg.Hosts()
	if len(hostnames) == 0 {
		fmt.Fprintf(stderr,
			"You are not logged into any GitHub hosts. Run %s to authenticate.\n", cs.Bold("gh auth login"))
		return cmdutil.SilentError
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	var failed bool
	var isHostnameFound bool

	for _, hostname := range hostnames {
		if opts.Hostname != "" && opts.Hostname != hostname {
			continue
		}
		isHostnameFound = true

		token, tokenSource := authCfg.Token(hostname)
		if tokenSource == "oauth_token" {
			// The go-gh function TokenForHost returns this value as source for tokens read from the
			// config file, but we want the file path instead. This attempts to reconstruct it.
			tokenSource = filepath.Join(config.ConfigDir(), "hosts.yml")
		}
		_, tokenIsWriteable := shared.AuthTokenWriteable(authCfg, hostname)

		statusInfo[hostname] = []string{}
		addMsg := func(x string, ys ...interface{}) {
			statusInfo[hostname] = append(statusInfo[hostname], fmt.Sprintf(x, ys...))
		}

		scopesHeader, err := shared.GetScopes(httpClient, hostname, token)
		if err != nil {
			addMsg("%s %s: authentication failed", cs.Red("X"), hostname)
			addMsg("- The %s token in %s is no longer valid.", cs.Bold(hostname), tokenSource)
			if tokenIsWriteable {
				addMsg("- To re-authenticate, run: %s %s",
					cs.Bold("gh auth login -h"), cs.Bold(hostname))
				addMsg("- To forget about this host, run: %s %s",
					cs.Bold("gh auth logout -h"), cs.Bold(hostname))
			}
			failed = true
			continue
		}

		if err := shared.HeaderHasMinimumScopes(scopesHeader); err != nil {
			var missingScopes *shared.MissingScopesError
			if errors.As(err, &missingScopes) {
				addMsg("%s %s: the token in %s is %s", cs.Red("X"), hostname, tokenSource, err)
				if tokenIsWriteable {
					addMsg("- To request missing scopes, run: %s %s",
						cs.Bold("gh auth refresh -h"),
						cs.Bold(hostname))
				}
			}
			failed = true
		} else {
			apiClient := api.NewClientFromHTTP(httpClient)
			username, err := api.CurrentLoginName(apiClient, hostname)
			if err != nil {
				addMsg("%s %s: api call failed: %s", cs.Red("X"), hostname, err)
				failed = true
			}

			addMsg("%s Logged in to %s as %s (%s)", cs.SuccessIcon(), hostname, cs.Bold(username), tokenSource)
			proto, _ := authCfg.GitProtocol(hostname)
			if proto != "" {
				addMsg("%s Git operations for %s configured to use %s protocol.",
					cs.SuccessIcon(), hostname, cs.Bold(proto))
			}
			addMsg("%s Token: %s", cs.SuccessIcon(), displayToken(token, opts.ShowToken))

			if scopesHeader != "" {
				addMsg("%s Token scopes: %s", cs.SuccessIcon(), scopesHeader)
			} else if expectScopes(token) {
				addMsg("%s Token scopes: none", cs.Red("X"))
			}
		}
	}

	if !isHostnameFound {
		fmt.Fprintf(stderr,
			"Hostname %q not found among authenticated GitHub hosts\n", opts.Hostname)
		return cmdutil.SilentError
	}

	prevEntry := false
	for _, hostname := range hostnames {
		lines, ok := statusInfo[hostname]
		if !ok {
			continue
		}
		if prevEntry {
			fmt.Fprint(stderr, "\n")
		}
		prevEntry = true
		fmt.Fprintf(stderr, "%s\n", cs.Bold(hostname))
		for _, line := range lines {
			fmt.Fprintf(stderr, "  %s\n", line)
		}
	}

	if failed {
		return cmdutil.SilentError
	}

	return nil
}

func displayToken(token string, printRaw bool) string {
	if printRaw {
		return token
	}

	if idx := strings.LastIndexByte(token, '_'); idx > -1 {
		prefix := token[0 : idx+1]
		return prefix + strings.Repeat("*", len(token)-len(prefix))
	}

	return strings.Repeat("*", len(token))
}

func expectScopes(token string) bool {
	return strings.HasPrefix(token, "ghp_") || strings.HasPrefix(token, "gho_")
}
