package token

import (
	"errors"
	"fmt"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type TokenOptions struct {
	IO     *iostreams.IOStreams
	Config func() (gh.Config, error)

	Hostname      string
	Username      string
	SecureStorage bool
	NoRefresh     bool
}

func NewCmdToken(f *cmdutil.Factory, runF func(*TokenOptions) error) *cobra.Command {
	opts := &TokenOptions{
		IO:     f.IOStreams,
		Config: f.Config,
	}

	cmd := &cobra.Command{
		Use:   "token",
		Short: "Print the authentication token gh uses for a hostname and account",
		Long: heredoc.Docf(`
			This command outputs the authentication token for an account on a given GitHub host.

			Without the %[1]s--hostname%[1]s flag, the default host is chosen.

			Without the %[1]s--user%[1]s flag, the active account for the host is chosen.

			If the token is an expired or near-expiry token that supports refreshing, it is refreshed
			automatically before being printed, unless %[1]s--no-refresh%[1]s is given.
		`, "`"),
		Args: cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}

			return tokenRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Hostname, "hostname", "h", "", "The hostname of the GitHub instance authenticated with")
	cmd.Flags().StringVarP(&opts.Username, "user", "u", "", "The account to output the token for")
	cmd.Flags().BoolVarP(&opts.SecureStorage, "secure-storage", "", false, "Search only secure credential store for authentication token")
	_ = cmd.Flags().MarkHidden("secure-storage")
	cmd.Flags().BoolVarP(&opts.NoRefresh, "no-refresh", "", false, "Do not automatically refresh an expired token")

	return cmd
}

func tokenRun(opts *TokenOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}
	authCfg := cfg.Authentication()
	cs := opts.IO.ColorScheme()

	hostname := opts.Hostname
	if hostname == "" {
		hostname, _ = authCfg.DefaultHost()
	}

	var val string
	var cred gh.Credential
	var refreshStatus gh.RefreshStatus
	switch {
	// No flags: return the active token, refreshing it when necessary, from wherever it is stored.
	case !opts.SecureStorage && !opts.NoRefresh:
		cred, refreshStatus = getTokenWithRefresh(authCfg, hostname, opts.Username)
		val = cred.Token

	// --no-refresh: return the stored active token from wherever it is stored, without refreshing.
	case !opts.SecureStorage && opts.NoRefresh:
		cred = getTokenReadOnly(authCfg, hostname, opts.Username)
		val = cred.Token

	// --secure-storage: compatibility mode for go-gh, which cannot read gh's storage directly. A refreshable token
	// is returned regardless of where it is stored, because gh owns its lifecycle and a go-gh based caller cannot
	// obtain it any other way. Otherwise this reproduces the original behaviour of the auth token command under
	// --secure-storage: read the token straight from the keyring and consult no environment variable or config
	// entry at all.
	case opts.SecureStorage && !opts.NoRefresh:
		// We resolve a username for the refresh lookup because a refreshable token is always stored under a user,
		// and ActiveTokenWithRefresh cannot be used upfront: with no username it resolves the active token, which an
		// environment variable such as GH_TOKEN can shadow, defeating --secure-storage.
		user := resolveUsername(authCfg, hostname, opts.Username)
		var c gh.Credential
		if user != "" {
			c, refreshStatus = getTokenWithRefresh(authCfg, hostname, user)
		}
		if c.IsRefreshable() {
			cred = c
			val = c.Token
		} else {
			// Non-refreshable fallback: read the keyring keyed on the actual flag value, so an omitted --user reads
			// the legacy unkeyed slot exactly as the original --secure-storage behaviour did, not a per-user slot.
			val = getNonRefreshableTokenFromKeyring(authCfg, hostname, opts.Username)
		}

	// --secure-storage --no-refresh: rare combination with no known caller. Respect --secure-storage strictly and
	// return the token only when it actually came from the keyring, otherwise nothing. The nature of the credential
	// (refreshable or not) is irrelevant here because refreshing was also declined, so there is no lifecycle to
	// honor; whoever passed --secure-storage wants the securely stored token specifically, so that is respected.
	case opts.SecureStorage && opts.NoRefresh:
		// We read under the resolved user so a refreshable token stored in the keyring is still surfaced, but only
		// return it when it genuinely came from the keyring. ActiveToken cannot be used upfront because an
		// environment variable such as GH_TOKEN can shadow the active token.
		user := resolveUsername(authCfg, hostname, opts.Username)
		var c gh.Credential
		if user != "" {
			c = getTokenReadOnly(authCfg, hostname, user)
		}
		if c.Source == gh.TokenSourceKeyring {
			cred = c
			val = c.Token
		} else {
			// Non-refreshable fallback: read the keyring keyed on the actual flag value, so an omitted --user reads
			// the legacy unkeyed slot exactly as the original --secure-storage behaviour did, not a per-user slot.
			val = getNonRefreshableTokenFromKeyring(authCfg, hostname, opts.Username)
		}
	}

	// A rejected refresh token can only be recovered by re-authenticating, so surface that as an error rather than
	// handing back the stale token, which would fail on the next API call anyway.
	if refreshStatus == gh.RefreshStatusExpired {
		errMsg := fmt.Sprintf("the token for %s has expired", hostname)
		if opts.Username != "" {
			errMsg += fmt.Sprintf(" for account %s", opts.Username)
		}
		errMsg += "; please run 'gh auth login' to re-authenticate"
		return errors.New(errMsg)
	}

	if val == "" {
		errMsg := fmt.Sprintf("no oauth token found for %s", hostname)
		if opts.Username != "" {
			errMsg += fmt.Sprintf(" account %s", opts.Username)
		}
		return errors.New(errMsg)
	}

	// The token is written to stdout as a script contract, so this caution goes to stderr only, and before the token
	// so it stays visible even when stdout is redirected. A refreshable token is short-lived: gh may rotate it at any
	// time, so a value captured now can stop working before its nominal expiry.
	if cred.IsRefreshable() {
		if cred.ExpiresAt != nil {
			fmt.Fprintf(opts.IO.ErrOut, "%s This token will expire on %s, or may be revoked before that time as a result of an expected refresh.\n", cs.WarningIcon(), cred.ExpiresAt.UTC().Format(time.RFC3339))
		} else {
			fmt.Fprintf(opts.IO.ErrOut, "%s This token will expire, or may be revoked at any time as a result of an expected refresh.\n", cs.WarningIcon())
		}
	}

	fmt.Fprintf(opts.IO.Out, "%s\n", val)

	return nil
}

func resolveUsername(authCfg gh.AuthConfig, hostname, username string) string {
	if username != "" {
		return username
	}
	user, _ := authCfg.ActiveUser(hostname)
	return user
}

// getTokenReadOnly resolves the stored active token for the hostname and optional username without contacting the
// token endpoint. It still surfaces a refreshable credential's stored access token, so an expiring token is
// returned, just not refreshed.
func getTokenReadOnly(authCfg gh.AuthConfig, hostname, username string) gh.Credential {
	if username == "" {
		return authCfg.ActiveToken(hostname)
	}
	cred, _ := authCfg.TokenForUser(hostname, username)
	return cred
}

// getTokenWithRefresh resolves the active token for the hostname and optional username and refreshes it when
// necessary. Refresh is best effort: a failed refresh still yields the last stored token, and an empty result is
// left for the caller to handle.
func getTokenWithRefresh(authCfg gh.AuthConfig, hostname, username string) (gh.Credential, gh.RefreshStatus) {
	if username == "" {
		cred, status, _ := authCfg.ActiveTokenWithRefresh(hostname)
		return cred, status
	}
	cred, status, _ := authCfg.TokenForUserWithRefresh(hostname, username)
	return cred, status
}

// getNonRefreshableTokenFromKeyring reads the token straight from secure storage for the hostname and optional
// username, consulting no environment variable or config entry.
func getNonRefreshableTokenFromKeyring(authCfg gh.AuthConfig, hostname, username string) string {
	if username == "" {
		token, _ := authCfg.TokenFromKeyring(hostname)
		return token
	}
	token, _ := authCfg.TokenFromKeyringForUser(hostname, username)
	return token
}
