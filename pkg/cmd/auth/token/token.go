package token

import (
	"errors"
	"fmt"

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

			If the token is an expired or near-expiry short-lived token that supports refreshing, it is refreshed
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
	cmd.Flags().BoolVarP(&opts.NoRefresh, "no-refresh", "", false, "Do not automatically refresh an expired short-lived token")

	return cmd
}

func tokenRun(opts *TokenOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}
	authCfg := cfg.Authentication()

	hostname := opts.Hostname
	if hostname == "" {
		hostname, _ = authCfg.DefaultHost()
	}

	var val string
	var refreshStatus gh.RefreshStatus
	switch {
	// No flags: return the active token, refreshing it when necessary, from wherever it is stored.
	case !opts.SecureStorage && !opts.NoRefresh:
		var cred gh.Credential
		cred, refreshStatus = getTokenWithRefresh(authCfg, hostname, opts.Username)
		val = cred.Token

	// --no-refresh: return the stored active token from wherever it is stored, without refreshing.
	case !opts.SecureStorage && opts.NoRefresh:
		val = getTokenReadOnly(authCfg, hostname, opts.Username).Token

	// --secure-storage: compatibility mode for go-gh, which cannot read gh's storage directly. A refreshable token
	// is returned regardless of where it is stored, because gh owns its lifecycle and a go-gh based caller cannot
	// obtain it any other way; a token that was just refreshed is likewise returned as is, so a freshly minted
	// (possibly no longer refreshable) token is not silently dropped. Otherwise this reproduces the original
	// behaviour of the auth token command under --secure-storage: read the token straight from the keyring and
	// consult no environment variable or config entry at all.
	case opts.SecureStorage && !opts.NoRefresh:
		// We cannot use ActiveToken or ActiveTokenWithRefresh upfront here: with no username they resolve the active
		// token, which an environment variable such as GH_TOKEN can shadow, defeating --secure-storage. So when no
		// username is given we resolve one and look the token up under it directly in the keyring. The refresh path
		// does not need this fallback because a refreshable token is always stored under a user.
		user := resolveUsername(authCfg, hostname, opts.Username)
		var cred gh.Credential
		if user != "" {
			cred, refreshStatus = getTokenWithRefresh(authCfg, hostname, user)
		}
		if cred.IsRefreshable() || refreshStatus == gh.RefreshStatusDone {
			val = cred.Token
		} else {
			val = getNonRefreshableTokenFromKeyring(authCfg, hostname, user)
		}

	// --secure-storage --no-refresh: rare combination with no known caller. Respect --secure-storage strictly and
	// return the token only when it actually came from the keyring, otherwise nothing. The nature of the credential
	// (refreshable or not) is irrelevant here because refreshing was also declined, so there is no lifecycle to
	// honor; whoever passed --secure-storage wants the securely stored token specifically, so that is respected.
	case opts.SecureStorage && opts.NoRefresh:
		// As in the case above, ActiveToken cannot be used upfront because an environment variable such as GH_TOKEN
		// can shadow the active token. So when no username is given we resolve one and read its keyring token
		// directly, falling back to the legacy unkeyed keyring slot when no active user is recorded.
		user := resolveUsername(authCfg, hostname, opts.Username)
		var cred gh.Credential
		if user != "" {
			cred = getTokenReadOnly(authCfg, hostname, user)
		} else if token, err := authCfg.TokenFromKeyring(hostname); err == nil {
			cred = gh.Credential{Source: gh.TokenSourceKeyring, Token: token}
		}
		if cred.Source == gh.TokenSourceKeyring {
			val = cred.Token
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
// token endpoint. It still surfaces a refreshable credential's stored access token, so a short-lived token is
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
