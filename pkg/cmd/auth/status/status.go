package status

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/cmd/auth/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type validEntry struct {
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
		fmt.Sprintf("  %s Logged in to %s as %s (%s)\n", cs.SuccessIcon(), e.host, cs.Bold(e.user), e.tokenSource),
	)
	if e.gitProtocol != "" {
		sb.WriteString(fmt.Sprintf("  %s Git operations for %s configured to use %s protocol.\n",
			cs.SuccessIcon(), e.host, cs.Bold(e.gitProtocol)))
	}
	sb.WriteString(fmt.Sprintf("  %s Token: %s\n", cs.SuccessIcon(), e.token))

	if e.scopes != "" {
		sb.WriteString(fmt.Sprintf("  %s Token scopes: %s\n", cs.SuccessIcon(), e.scopes))
	} else if expectScopes(e.token) {
		sb.WriteString(fmt.Sprintf("  %s Token scopes: none\n", cs.Red("X")))
	}

	return sb.String()
}

type missingScopes []string

func (ms missingScopes) String() string {
	var missing []string
	for _, s := range ms {
		missing = append(missing, fmt.Sprintf("'%s'", s))
	}
	scopes := strings.Join(missing, ", ")

	if len(ms) == 1 {
		return "missing required scope " + scopes
	}
	return "missing required scopes " + scopes
}

type missingScopesEntry struct {
	host             string
	tokenSource      string
	missingScopes    missingScopes
	tokenIsWriteable bool
}

func (e missingScopesEntry) String(cs *iostreams.ColorScheme) string {
	var sb strings.Builder

	sb.WriteString(
		fmt.Sprintf("  %s %s: the token in %s is %s\n", cs.Red("X"), e.host, e.tokenSource, e.missingScopes),
	)
	if e.tokenIsWriteable {
		sb.WriteString(fmt.Sprintf("  - To request missing scopes, run: %s %s\n",
			cs.Bold("gh auth refresh -h"),
			cs.Bold(e.host)))
	}

	return sb.String()
}

type invalidTokenEntry struct {
	host             string
	tokenSource      string
	tokenIsWriteable bool
}

func (e invalidTokenEntry) String(cs *iostreams.ColorScheme) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("  %s %s: authentication failed\n", cs.Red("X"), e.host))
	sb.WriteString(fmt.Sprintf("  - The %s token in %s is invalid.\n", cs.Bold(e.host), e.tokenSource))
	if e.tokenIsWriteable {
		sb.WriteString(fmt.Sprintf("  - To re-authenticate, run: %s %s\n",
			cs.Bold("gh auth login -h"), cs.Bold(e.host)))
		sb.WriteString(fmt.Sprintf("  - To forget about this host, run: %s %s\n",
			cs.Bold("gh auth logout -h"), cs.Bold(e.host)))
	}

	return sb.String()
}

type timeoutErrorEntry struct {
	host string
}

func (e timeoutErrorEntry) String(cs *iostreams.ColorScheme) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("  %s %s: timeout trying to connect to host\n", cs.Red("X"), e.host))

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
	stdout := opts.IO.Out
	cs := opts.IO.ColorScheme()

	statuses := make(map[string]Entries)

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

	var isHostnameFound bool

	for _, hostname := range hostnames {
		if opts.Hostname != "" && opts.Hostname != hostname {
			continue
		}
		isHostnameFound = true

		users, _ := authCfg.UsersForHost(hostname)
		for _, username := range users {
			token, tokenSource, _ := authCfg.TokenForUser(hostname, username)
			if tokenSource == "oauth_token" {
				// The go-gh function TokenForHost returns this value as source for tokens read from the
				// config file, but we want the file path instead. This attempts to reconstruct it.
				tokenSource = filepath.Join(config.ConfigDir(), "hosts.yml")
			}
			_, tokenIsWriteable := shared.AuthTokenWriteable(authCfg, hostname)

			scopesHeader, err := shared.GetScopes(httpClient, hostname, token)
			if err != nil {
				var networkError net.Error
				if errors.As(err, &networkError) && networkError.Timeout() {
					statuses[hostname] = append(statuses[hostname], timeoutErrorEntry{
						host: hostname,
					})
				} else {
					statuses[hostname] = append(statuses[hostname], invalidTokenEntry{
						host:             hostname,
						tokenSource:      tokenSource,
						tokenIsWriteable: tokenIsWriteable,
					})
				}

				continue
			}

			if err := shared.HeaderHasMinimumScopes(scopesHeader); err != nil {
				var missingScopes *shared.MissingScopesError
				if errors.As(err, &missingScopes) {
					statuses[hostname] = append(statuses[hostname], missingScopesEntry{
						host:             hostname,
						tokenSource:      tokenSource,
						missingScopes:    missingScopes.MissingScopes,
						tokenIsWriteable: tokenIsWriteable,
					})
				}
			} else {
				statuses[hostname] = append(statuses[hostname], validEntry{
					host:        hostname,
					user:        username,
					token:       displayToken(token, opts.ShowToken),
					tokenSource: tokenSource,
					gitProtocol: cfg.GitProtocol(hostname),
					scopes:      scopesHeader})
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
		entries, ok := statuses[hostname]
		if !ok {
			continue
		}

		if prevEntry {
			fmt.Fprint(stdout, "\n")
		}
		prevEntry = true
		fmt.Fprintf(stdout, "%s\n", cs.Bold(hostname))
		fmt.Fprintf(stdout, "%s", strings.Join(entries.Strings(cs), "\n"))
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
