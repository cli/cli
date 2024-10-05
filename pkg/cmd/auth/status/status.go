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

type validEntry struct {
	active      bool
	host        string
	user        string
	token       string
	tokenSource string
	gitProtocol string
	scopes      string
}

func (e validEntry) String(cs *iostreams.ColorScheme) string {
	var sb strings.Builder

	sb.WriteString(
		fmt.Sprintf("  %s Logged in to %s account %s (%s)\n", cs.SuccessIcon(), e.host, cs.Bold(e.user), e.tokenSource),
	)
	activeStr := fmt.Sprintf("%v", e.active)
	sb.WriteString(fmt.Sprintf("  - Active account: %s\n", cs.Bold(activeStr)))
	sb.WriteString(fmt.Sprintf("  - Git operations protocol: %s\n", cs.Bold(e.gitProtocol)))
	sb.WriteString(fmt.Sprintf("  - Token: %s\n", cs.Bold(e.token)))

	if expectScopes(e.token) {
		sb.WriteString(fmt.Sprintf("  - Token scopes: %s\n", cs.Bold(displayScopes(e.scopes))))
		if err := shared.HeaderHasMinimumScopes(e.scopes); err != nil {
			var missingScopesError *shared.MissingScopesError
			if errors.As(err, &missingScopesError) {
				missingScopes := strings.Join(missingScopesError.MissingScopes, ",")
				sb.WriteString(fmt.Sprintf("  %s Missing required token scopes: %s\n",
					cs.WarningIcon(),
					cs.Bold(displayScopes(missingScopes))))
				refreshInstructions := fmt.Sprintf("gh auth refresh -h %s", e.host)
				sb.WriteString(fmt.Sprintf("  - To request missing scopes, run: %s\n", cs.Bold(refreshInstructions)))
			}
		}
	}

	return sb.String()
}

type invalidTokenEntry struct {
	active           bool
	host             string
	user             string
	tokenSource      string
	tokenIsWriteable bool
}

func (e invalidTokenEntry) String(cs *iostreams.ColorScheme) string {
	var sb strings.Builder

	if e.user != "" {
		sb.WriteString(fmt.Sprintf("  %s Failed to log in to %s account %s (%s)\n", cs.Red("X"), e.host, cs.Bold(e.user), e.tokenSource))
	} else {
		sb.WriteString(fmt.Sprintf("  %s Failed to log in to %s using token (%s)\n", cs.Red("X"), e.host, e.tokenSource))
	}
	activeStr := fmt.Sprintf("%v", e.active)
	sb.WriteString(fmt.Sprintf("  - Active account: %s\n", cs.Bold(activeStr)))
	sb.WriteString(fmt.Sprintf("  - The token in %s is invalid.\n", e.tokenSource))
	if e.tokenIsWriteable {
		loginInstructions := fmt.Sprintf("gh auth login -h %s", e.host)
		logoutInstructions := fmt.Sprintf("gh auth logout -h %s -u %s", e.host, e.user)
		sb.WriteString(fmt.Sprintf("  - To re-authenticate, run: %s\n", cs.Bold(loginInstructions)))
		sb.WriteString(fmt.Sprintf("  - To forget about this account, run: %s\n", cs.Bold(logoutInstructions)))
	}

	return sb.String()
}

type timeoutErrorEntry struct {
	active      bool
	host        string
	user        string
	tokenSource string
}

func (e timeoutErrorEntry) String(cs *iostreams.ColorScheme) string {
	var sb strings.Builder

	if e.user != "" {
		sb.WriteString(fmt.Sprintf("  %s Timeout trying to log in to %s account %s (%s)\n", cs.Red("X"), e.host, cs.Bold(e.user), e.tokenSource))
	} else {
		sb.WriteString(fmt.Sprintf("  %s Timeout trying to log in to %s using token (%s)\n", cs.Red("X"), e.host, e.tokenSource))
	}
	activeStr := fmt.Sprintf("%v", e.active)
	sb.WriteString(fmt.Sprintf("  - Active account: %s\n", cs.Bold(activeStr)))

	return sb.String()
}

type Entry interface {
	String(cs *iostreams.ColorScheme) string
}

type Entries []Entry

func (e Entries) Strings(cs *iostreams.ColorScheme) []string {
	var out []string
	for _, entry := range e {
		out = append(out, entry.String(cs))
	}
	return out
}

type StatusOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	Config     func() (gh.Config, error)

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
			the command will exit with 1 and output to stderr.

			To change the active account for a host, see %[1]sgh auth switch%[1]s.
		`, "`"),
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

	statuses := make(map[string]Entries)

	hostnames := authCfg.Hosts()
	if len(hostnames) == 0 {
		fmt.Fprintf(stderr,
			"You are not logged into any GitHub hosts. To log in, run: %s\n", cs.Bold("gh auth login"))
		return cmdutil.SilentError
	}

	if opts.Hostname != "" && !slices.Contains(hostnames, opts.Hostname) {
		fmt.Fprintf(stderr,
			"You are not logged into any accounts on %s\n", opts.Hostname)
		return cmdutil.SilentError
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

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
			showToken:   opts.ShowToken,
			token:       activeUserToken,
			tokenSource: activeUserTokenSource,
			username:    activeUser,
		})
		statuses[hostname] = append(statuses[hostname], entry)

		if err == nil && !isValidEntry(entry) {
			err = cmdutil.SilentError
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
				showToken:   opts.ShowToken,
				token:       token,
				tokenSource: tokenSource,
				username:    username,
			})
			statuses[hostname] = append(statuses[hostname], entry)

			if err == nil && !isValidEntry(entry) {
				err = cmdutil.SilentError
			}
		}
	}

	prevEntry := false
	for _, hostname := range hostnames {
		entries, ok := statuses[hostname]
		if !ok {
			continue
		}

		stream := stdout
		if err != nil {
			stream = stderr
		}

		if prevEntry {
			fmt.Fprint(stream, "\n")
		}
		prevEntry = true
		fmt.Fprintf(stream, "%s\n", cs.Bold(hostname))
		fmt.Fprintf(stream, "%s", strings.Join(entries.Strings(cs), "\n"))
	}

	return err
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
	showToken   bool
	token       string
	tokenSource string
	username    string
}

func buildEntry(httpClient *http.Client, opts buildEntryOptions) Entry {
	tokenIsWriteable := authTokenWriteable(opts.tokenSource)

	if opts.tokenSource == "oauth_token" {
		// The go-gh function TokenForHost returns this value as source for tokens read from the
		// config file, but we want the file path instead. This attempts to reconstruct it.
		opts.tokenSource = filepath.Join(config.ConfigDir(), "hosts.yml")
	}

	// If token is not writeable, then it came from an environment variable and
	// we need to fetch the username as it won't be stored in the config.
	if !tokenIsWriteable {
		// The httpClient will automatically use the correct token here as
		// the token from the environment variable take highest precedence.
		apiClient := api.NewClientFromHTTP(httpClient)
		var err error
		opts.username, err = api.CurrentLoginName(apiClient, opts.hostname)
		if err != nil {
			return invalidTokenEntry{
				active:           opts.active,
				host:             opts.hostname,
				user:             opts.username,
				tokenIsWriteable: tokenIsWriteable,
				tokenSource:      opts.tokenSource,
			}
		}
	}

	// Get scopes for token.
	scopesHeader, err := shared.GetScopes(httpClient, opts.hostname, opts.token)
	if err != nil {
		var networkError net.Error
		if errors.As(err, &networkError) && networkError.Timeout() {
			return timeoutErrorEntry{
				active:      opts.active,
				host:        opts.hostname,
				user:        opts.username,
				tokenSource: opts.tokenSource,
			}
		}

		return invalidTokenEntry{
			active:           opts.active,
			host:             opts.hostname,
			user:             opts.username,
			tokenIsWriteable: tokenIsWriteable,
			tokenSource:      opts.tokenSource,
		}
	}

	return validEntry{
		active:      opts.active,
		gitProtocol: opts.gitProtocol,
		host:        opts.hostname,
		scopes:      scopesHeader,
		token:       displayToken(opts.token, opts.showToken),
		tokenSource: opts.tokenSource,
		user:        opts.username,
	}
}

func authTokenWriteable(src string) bool {
	return !strings.HasSuffix(src, "_TOKEN")
}

func isValidEntry(entry Entry) bool {
	_, ok := entry.(validEntry)
	return ok
}
