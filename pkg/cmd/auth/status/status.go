package status

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"slices"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/cmd/auth/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type authEntryState string

const (
	authEntryStateSuccess = "success"
	authEntryStateTimeout = "timeout"
	authEntryStateError   = "error"
)

type authEntry struct {
	State       authEntryState `json:"state"`
	Error       string         `json:"error,omitempty"`
	Active      bool           `json:"active"`
	Host        string         `json:"host"`
	Login       string         `json:"login"`
	TokenSource string         `json:"tokenSource"`
	Token       string         `json:"token,omitempty"`
	Scopes      string         `json:"scopes,omitempty"`
	GitProtocol string         `json:"gitProtocol"`
}

type authStatus struct {
	Hosts map[string][]authEntry `json:"hosts"`
}

func newAuthStatus() *authStatus {
	return &authStatus{
		Hosts: make(map[string][]authEntry),
	}
}

var authStatusFields = []string{
	"hosts",
}

func (a authStatus) ExportData(fields []string) map[string]interface{} {
	return cmdutil.StructExportData(a, fields)
}

func (e authEntry) String(cs *iostreams.ColorScheme) string {
	var sb strings.Builder

	switch e.State {
	case authEntryStateSuccess:
		sb.WriteString(
			fmt.Sprintf("  %s Logged in to %s account %s (%s)\n", cs.SuccessIcon(), e.Host, cs.Bold(e.Login), e.TokenSource),
		)
		activeStr := fmt.Sprintf("%v", e.Active)
		sb.WriteString(fmt.Sprintf("  - Active account: %s\n", cs.Bold(activeStr)))
		sb.WriteString(fmt.Sprintf("  - Git operations protocol: %s\n", cs.Bold(e.GitProtocol)))
		sb.WriteString(fmt.Sprintf("  - Token: %s\n", cs.Bold(e.Token)))

		if expectScopes(e.Token) {
			sb.WriteString(fmt.Sprintf("  - Token scopes: %s\n", cs.Bold(displayScopes(e.Scopes))))
			if err := shared.HeaderHasMinimumScopes(e.Scopes); err != nil {
				var missingScopesError *shared.MissingScopesError
				if errors.As(err, &missingScopesError) {
					missingScopes := strings.Join(missingScopesError.MissingScopes, ",")
					sb.WriteString(fmt.Sprintf("  %s Missing required token scopes: %s\n",
						cs.WarningIcon(),
						cs.Bold(displayScopes(missingScopes))))
					refreshInstructions := fmt.Sprintf("gh auth refresh -h %s", e.Host)
					sb.WriteString(fmt.Sprintf("  - To request missing scopes, run: %s\n", cs.Bold(refreshInstructions)))
				}
			}
		}

	case authEntryStateError:
		if e.Login != "" {
			sb.WriteString(fmt.Sprintf("  %s Failed to log in to %s account %s (%s)\n", cs.Red("X"), e.Host, cs.Bold(e.Login), e.TokenSource))
		} else {
			sb.WriteString(fmt.Sprintf("  %s Failed to log in to %s using token (%s)\n", cs.Red("X"), e.Host, e.TokenSource))
		}
		activeStr := fmt.Sprintf("%v", e.Active)
		sb.WriteString(fmt.Sprintf("  - Active account: %s\n", cs.Bold(activeStr)))
		sb.WriteString(fmt.Sprintf("  - The token in %s is invalid.\n", e.TokenSource))
		if authTokenWriteable(e.TokenSource) {
			loginInstructions := fmt.Sprintf("gh auth login -h %s", e.Host)
			logoutInstructions := fmt.Sprintf("gh auth logout -h %s -u %s", e.Host, e.Login)
			sb.WriteString(fmt.Sprintf("  - To re-authenticate, run: %s\n", cs.Bold(loginInstructions)))
			sb.WriteString(fmt.Sprintf("  - To forget about this account, run: %s\n", cs.Bold(logoutInstructions)))
		}

	case authEntryStateTimeout:
		if e.Login != "" {
			sb.WriteString(fmt.Sprintf("  %s Timeout trying to log in to %s account %s (%s)\n", cs.Red("X"), e.Host, cs.Bold(e.Login), e.TokenSource))
		} else {
			sb.WriteString(fmt.Sprintf("  %s Timeout trying to log in to %s using token (%s)\n", cs.Red("X"), e.Host, e.TokenSource))
		}
		activeStr := fmt.Sprintf("%v", e.Active)
		sb.WriteString(fmt.Sprintf("  - Active account: %s\n", cs.Bold(activeStr)))
	}

	return sb.String()
}

type StatusOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	Config     func() (gh.Config, error)
	Exporter   cmdutil.Exporter

	Hostname  string
	ShowToken bool
	Active    bool
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
		Short: "Display active account and authentication state on each known GitHub host",
		Long: heredoc.Docf(`
			Display active account and authentication state on each known GitHub host.

			For each host, the authentication state of each known account is tested and any issues are included in the output.
			Each host section will indicate the active account, which will be used when targeting that host.

			If an account on any host (or only the one given via %[1]s--hostname%[1]s) has authentication issues,
			the command will exit with 1 and output to stderr. Note that when using the %[1]s--json%[1]s option, the command
			will always exit with zero regardless of any authentication issues, unless there is a fatal error.

			To change the active account for a host, see %[1]sgh auth switch%[1]s.
		`, "`"),
		Example: heredoc.Doc(`
			# Display authentication status for all accounts on all hosts
			$ gh auth status

			# Display authentication status for the active account on a specific host
			$ gh auth status --active --hostname github.example.com
			
			# Display tokens in plain text
			$ gh auth status --show-token

			# Format authentication status as JSON
			$ gh auth status --json hosts

			# Include plain text token in JSON output
			$ gh auth status --json hosts --show-token

			# Format hosts as a flat JSON array
			$ gh auth status --json hosts --jq '.hosts | add'
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}

			return statusRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Hostname, "hostname", "h", "", "Check only a specific hostname's auth status")
	cmd.Flags().BoolVarP(&opts.ShowToken, "show-token", "t", false, "Display the auth token")
	cmd.Flags().BoolVarP(&opts.Active, "active", "a", false, "Display the active account only")

	// the json flags are intentionally not given a shorthand to avoid conflict with -t/--show-token
	cmdutil.AddJSONFlagsWithoutShorthand(cmd, &opts.Exporter, authStatusFields)

	return cmd
}

func statusRun(opts *StatusOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}
	authCfg := cfg.Authentication()

	stderr := opts.IO.ErrOut
	stdout := opts.IO.Out
	cs := opts.IO.ColorScheme()

	hostnames := authCfg.Hosts()
	if len(hostnames) == 0 {
		fmt.Fprintf(stderr,
			"You are not logged into any GitHub hosts. To log in, run: %s\n", cs.Bold("gh auth login"))
		if opts.Exporter != nil {
			// In machine-friendly mode, we always exit with no error.
			opts.Exporter.Write(opts.IO, newAuthStatus())
			return nil
		}
		return cmdutil.ErrSilent
	}

	if opts.Hostname != "" && !slices.Contains(hostnames, opts.Hostname) {
		fmt.Fprintf(stderr,
			"You are not logged into any accounts on %s\n", opts.Hostname)
		if opts.Exporter != nil {
			// In machine-friendly mode, we always exit with no error.
			opts.Exporter.Write(opts.IO, newAuthStatus())
			return nil
		}
		return cmdutil.ErrSilent
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	var finalErr error
	statuses := newAuthStatus()

	for _, hostname := range hostnames {
		if opts.Hostname != "" && opts.Hostname != hostname {
			continue
		}

		var activeUser string
		gitProtocol := cfg.GitProtocol(hostname).Value
		activeUserToken, activeUserTokenSource := authCfg.ActiveToken(hostname)
		if authTokenWriteable(activeUserTokenSource) {
			activeUser, _ = authCfg.ActiveUser(hostname)
		}
		entry := buildEntry(httpClient, buildEntryOptions{
			active:      true,
			gitProtocol: gitProtocol,
			hostname:    hostname,
			token:       activeUserToken,
			tokenSource: activeUserTokenSource,
			username:    activeUser,
		})
		statuses.Hosts[hostname] = append(statuses.Hosts[hostname], entry)

		if finalErr == nil && entry.State != authEntryStateSuccess {
			finalErr = cmdutil.ErrSilent
		}

		if opts.Active {
			continue
		}

		users := authCfg.UsersForHost(hostname)
		for _, username := range users {
			if username == activeUser {
				continue
			}
			token, tokenSource, _ := authCfg.TokenForUser(hostname, username)
			entry := buildEntry(httpClient, buildEntryOptions{
				active:      false,
				gitProtocol: gitProtocol,
				hostname:    hostname,
				token:       token,
				tokenSource: tokenSource,
				username:    username,
			})
			statuses.Hosts[hostname] = append(statuses.Hosts[hostname], entry)

			if finalErr == nil && entry.State != authEntryStateSuccess {
				finalErr = cmdutil.ErrSilent
			}
		}
	}

	if !opts.ShowToken {
		for _, host := range statuses.Hosts {
			for i := range host {
				if opts.Exporter != nil {
					// In machine-readable we just drop the token
					host[i].Token = ""
				} else {
					host[i].Token = maskToken(host[i].Token)
				}
			}
		}
	}

	if opts.Exporter != nil {
		// In machine-friendly mode, we always exit with no error.
		opts.Exporter.Write(opts.IO, statuses)
		return nil
	}

	prevEntry := false
	for _, hostname := range hostnames {
		entries, ok := statuses.Hosts[hostname]
		if !ok {
			continue
		}

		stream := stdout
		if finalErr != nil {
			stream = stderr
		}

		if prevEntry {
			fmt.Fprint(stream, "\n")
		}
		prevEntry = true
		fmt.Fprintf(stream, "%s\n", cs.Bold(hostname))
		for i, entry := range entries {
			fmt.Fprintf(stream, "%s", entry.String(cs))
			if i < len(entries)-1 {
				fmt.Fprint(stream, "\n")
			}
		}
	}

	return finalErr
}

func maskToken(token string) string {
	if idx := strings.LastIndexByte(token, '_'); idx > -1 {
		prefix := token[0 : idx+1]
		return prefix + strings.Repeat("*", len(token)-len(prefix))
	}
	return strings.Repeat("*", len(token))
}

func displayScopes(scopes string) string {
	if scopes == "" {
		return "none"
	}
	list := strings.Split(scopes, ",")
	for i, s := range list {
		list[i] = fmt.Sprintf("'%s'", strings.TrimSpace(s))
	}
	return strings.Join(list, ", ")
}

func expectScopes(token string) bool {
	return strings.HasPrefix(token, "ghp_") || strings.HasPrefix(token, "gho_")
}

type buildEntryOptions struct {
	active      bool
	gitProtocol string
	hostname    string
	token       string
	tokenSource string
	username    string
}

func buildEntry(httpClient *http.Client, opts buildEntryOptions) authEntry {
	tokenSource := opts.tokenSource
	if tokenSource == "oauth_token" {
		// The go-gh function TokenForHost returns this value as source for tokens read from the
		// config file, but we want the file path instead. This attempts to reconstruct it.
		tokenSource = filepath.Join(config.ConfigDir(), "hosts.yml")
	}
	entry := authEntry{
		Active:      opts.active,
		Host:        opts.hostname,
		Login:       opts.username,
		TokenSource: tokenSource,
		Token:       opts.token,
		GitProtocol: opts.gitProtocol,
	}

	// If token is not writeable, then it came from an environment variable and
	// we need to fetch the username as it won't be stored in the config.
	if !authTokenWriteable(tokenSource) {
		// The httpClient will automatically use the correct token here as
		// the token from the environment variable take highest precedence.
		apiClient := api.NewClientFromHTTP(httpClient)
		var err error
		entry.Login, err = api.CurrentLoginName(apiClient, opts.hostname)
		if err != nil {
			entry.State = authEntryStateError
			entry.Error = err.Error()
			return entry
		}
	}

	// Get scopes for token.
	scopesHeader, err := shared.GetScopes(httpClient, opts.hostname, opts.token)
	if err != nil {
		var networkError net.Error
		if errors.As(err, &networkError) && networkError.Timeout() {
			entry.State = authEntryStateTimeout
			entry.Error = err.Error()
			return entry
		}

		entry.State = authEntryStateError
		entry.Error = err.Error()
		return entry
	}
	entry.Scopes = scopesHeader

	entry.State = authEntryStateSuccess
	return entry
}

func authTokenWriteable(src string) bool {
	return !strings.HasSuffix(src, "_TOKEN")
}
