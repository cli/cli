package shared

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/authflow"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/gh/ghtelemetry"
	"github.com/cli/cli/v2/pkg/cmd/ssh-key/add"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/ssh"
	ghauth "github.com/cli/go-gh/v2/pkg/auth"
)

const defaultSSHKeyTitle = "GitHub CLI"

type iconfig interface {
	Login(string, string, string, string, bool) (bool, error)
	LoginRefreshable(string, string, gh.Credential, string, bool) (bool, error)
	UsersForHost(string) []string
}

type LoginOptions struct {
	IO               *iostreams.IOStreams
	Config           iconfig
	HTTPClient       *http.Client
	PlainHTTPClient  *http.Client
	Hostname         string
	Interactive      bool
	Web              bool
	Scopes           []string
	GitProtocol      string
	Prompter         Prompt
	Browser          browser.Browser
	CredentialFlow   *GitCredentialFlow
	SecureStorage    bool
	SkipSSHKeyPrompt bool
	CopyToClipboard  bool
	Expiring         bool

	// ExpiringExplicitlyRequested reports that the user passed --expiring to force expiring credentials. A pasted
	// token can never be expiring, so the paste option is suppressed only in this case. A config-derived preference
	// (expiring_credentials enabled) does not suppress paste: choosing it simply yields a non-expiring token.
	ExpiringExplicitlyRequested bool

	Telemetry ghtelemetry.InvocationRecorder

	// AuthFlow is indirected so tests can stub the interactive authorization. When nil, authflow.AuthFlow is used.
	AuthFlow func(*http.Client, string, *iostreams.IOStreams, string, []string, bool, browser.Browser, bool, bool) (*authflow.AuthResult, error)

	sshContext ssh.Context
}

// ShouldCopyToClipboard resolves an explicit flag value before falling back to configuration.
func ShouldCopyToClipboard(cfg gh.Config, flagValue *bool) bool {
	if flagValue != nil {
		return *flagValue
	}
	return cfg.Clipboard().Value == "enabled"
}

// ShouldUseExpiring resolves whether to prefer expiring credentials that gh refreshes automatically. An explicit
// --expiring flag value wins; when the flag is unset it falls back to the expiring_credentials config setting
// for the host.
func ShouldUseExpiring(cfg gh.Config, flagValue *bool, hostname string) bool {
	if flagValue != nil {
		return *flagValue
	}
	return cfg.ExpiringCredentials(hostname).Value == "enabled"
}

func Login(opts *LoginOptions) error {
	cfg := opts.Config
	hostname := opts.Hostname
	httpClient := opts.HTTPClient
	cs := opts.IO.ColorScheme()

	// Choose how to authenticate before anything that depends on expiring intent. A pasted token can never be
	// expiring, so the effective intent is only known once the method is chosen.
	var authMode int
	if opts.Web || opts.ExpiringExplicitlyRequested {
		// When the user explicitly passes --expiring, force the web flow so the paste option cannot silently ignore
		// that request. A config-derived preference does not suppress paste: choosing it yields a non-expiring token.
		authMode = 0
	} else if opts.Interactive {
		options := []string{"Login with a web browser", "Paste an authentication token"}
		var err error
		authMode, err = opts.Prompter.Select(
			"How would you like to authenticate GitHub CLI?",
			options[0],
			options)
		if err != nil {
			return err
		}
	}
	expiring := opts.Expiring && authMode == 0

	// Begin the auth-state telemetry event. It is nil for disabled telemetry or enterprise hosts, and all event methods
	// are nil-tolerant, so the calls below stay unconditional.
	authEvent := BeginAuthTelemetry(opts.Telemetry, "login", hostname, expiring)

	// A GitHub Enterprise Server host may not support expiring tokens. Surface this once the hostname is known, from
	// interactive selection or the --hostname flag, so the user understands they may receive a non-expiring token.
	if expiring && ghauth.IsEnterprise(hostname) {
		fmt.Fprintf(opts.IO.ErrOut, "%s %s may not support expiring tokens\n", cs.WarningIcon(), hostname)
	}

	gitProtocol := strings.ToLower(opts.GitProtocol)
	if opts.Interactive && gitProtocol == "" {
		options := []string{
			"HTTPS",
			"SSH",
		}
		result, err := opts.Prompter.Select(
			"What is your preferred protocol for Git operations on this host?",
			options[0],
			options)
		if err != nil {
			return err
		}
		proto := options[result]
		gitProtocol = strings.ToLower(proto)
	}

	var additionalScopes []string

	// Record the selected git operations protocol for every sign-in, before any prompt.
	authEvent.RecordGitProtocol(gitProtocol)

	// Record the configured credential helper up front, before any prompt, so this telemetry is captured for every
	// sign-in, including non-interactive and SSH flows that never reach the prompt.
	authEvent.RecordHelperBefore(opts.CredentialFlow.DetectConfiguredHelper(hostname))

	if opts.Interactive && gitProtocol == "https" {
		err := opts.CredentialFlow.Prompt(hostname, expiring)
		if errors.Is(err, ErrExpiringSetupCredHelperCancelled) {
			authEvent.RecordExpiringSetupCancelled(true)
			return cmdutil.CancelError
		}
		if err != nil {
			return err
		}
		additionalScopes = append(additionalScopes, opts.CredentialFlow.Scopes()...)
	}

	var keyToUpload string
	keyTitle := defaultSSHKeyTitle
	if opts.Interactive && !opts.SkipSSHKeyPrompt && gitProtocol == "ssh" {
		pubKeys, err := opts.sshContext.LocalPublicKeys()
		if err != nil {
			return err
		}

		if len(pubKeys) > 0 {
			options := append(pubKeys, "Skip")
			keyChoice, err := opts.Prompter.Select(
				"Upload your SSH public key to your GitHub account?",
				options[0],
				options)
			if err != nil {
				return err
			}
			if keyChoice < len(pubKeys) {
				keyToUpload = pubKeys[keyChoice]
			}
		} else if opts.sshContext.HasKeygen() {
			sshChoice, err := opts.Prompter.Confirm("Generate a new SSH key to add to your GitHub account?", true)
			if err != nil {
				return err
			}

			if sshChoice {
				passphrase, err := opts.Prompter.Password(
					"Enter a passphrase for your new SSH key (Optional):")
				if err != nil {
					return err
				}
				keyPair, err := opts.sshContext.GenerateSSHKey("id_ed25519", passphrase)
				if err != nil {
					return err
				}
				keyToUpload = keyPair.PublicKeyPath
			}
		}

		if keyToUpload != "" {
			var err error
			keyTitle, err = opts.Prompter.Input(
				"Title for your SSH key:", defaultSSHKeyTitle)
			if err != nil {
				return err
			}

			additionalScopes = append(additionalScopes, "admin:public_key")
		}
	}

	var authToken string
	var username string
	var refreshableCredential *gh.Credential

	if authMode == 0 {
		authFlow := opts.AuthFlow
		if authFlow == nil {
			authFlow = authflow.AuthFlow
		}
		result, err := authFlow(opts.PlainHTTPClient, hostname, opts.IO, "", append(opts.Scopes, additionalScopes...), opts.Interactive, opts.Browser, opts.CopyToClipboard, expiring)
		if err != nil {
			return fmt.Errorf("failed to authenticate via web browser: %w", err)
		}
		authToken = result.Token
		username = result.Username
		refreshableCredential = result.Refreshable
		fmt.Fprintf(opts.IO.ErrOut, "%s Authentication complete.\n", cs.SuccessIcon())
	} else {
		minimumScopes := append([]string{"repo", "read:org"}, additionalScopes...)
		fmt.Fprint(opts.IO.ErrOut, heredoc.Docf(`
			Tip: you can generate a Personal Access Token here https://%s/settings/tokens
			The minimum required scopes are %s.
		`, hostname, scopesSentence(minimumScopes)))

		var err error
		authToken, err = opts.Prompter.AuthToken()
		if err != nil {
			return err
		}

		if err := HasMinimumScopes(httpClient, hostname, authToken); err != nil {
			return fmt.Errorf("error validating token: %w", err)
		}
	}

	// Record the server's issuance outcome as soon as the auth flow returns. It is independent of local persistence, so
	// a later storage failure must not hide that an expiring, refreshable token was issued.
	authEvent.RecordExpiringReceived(refreshableCredential != nil)

	if username == "" {
		var err error
		username, err = GetCurrentLogin(httpClient, hostname, authToken)
		if err != nil {
			return fmt.Errorf("error retrieving current user: %w", err)
		}
	}

	// Get these users before adding the new one, so that we can
	// check whether the user was already logged in later.
	//
	// In this case we ignore the error if the host doesn't exist
	// because that can occur when the user is logging into a host
	// for the first time.
	usersForHost := cfg.UsersForHost(hostname)
	userWasAlreadyLoggedIn := slices.Contains(usersForHost, username)

	if gitProtocol != "" {
		fmt.Fprintf(opts.IO.ErrOut, "- gh config set -h %s git_protocol %s\n", hostname, gitProtocol)
		fmt.Fprintf(opts.IO.ErrOut, "%s Configured git protocol\n", cs.SuccessIcon())
	}

	var insecureStorageUsed bool
	var err error
	if refreshableCredential != nil {
		insecureStorageUsed, err = cfg.LoginRefreshable(hostname, username, *refreshableCredential, gitProtocol, opts.SecureStorage)
	} else {
		insecureStorageUsed, err = cfg.Login(hostname, username, authToken, gitProtocol, opts.SecureStorage)
	}
	if err != nil {
		return err
	}
	if insecureStorageUsed {
		fmt.Fprintf(opts.IO.ErrOut, "%s Authentication credentials saved in plain text\n", cs.Yellow("!"))
	}

	authEvent.RecordStorage(insecureStorageUsed)

	// Record success as soon as the credential is persisted. The git credential-helper setup and SSH key upload below
	// are separate side effects whose failure must not retract a credential that was already stored successfully.
	authEvent.RecordSuccess()

	// Expiring tokens are a preference, not a guarantee: the server decides based on its own and the OAuth app
	// configuration, so it may issue a refreshable token without being asked or a non-expiring one despite the request.
	switch {
	case refreshableCredential != nil && !expiring:
		fmt.Fprintf(opts.IO.ErrOut, "%s Host issues expiring refreshable token\n", cs.WarningIcon())
	case refreshableCredential != nil:
		fmt.Fprintf(opts.IO.ErrOut, "%s Received expiring refreshable token\n", cs.SuccessIcon())
	case expiring:
		fmt.Fprintf(opts.IO.ErrOut, "%s Host did not issue an expiring refreshable token\n", cs.WarningIcon())
	}

	if opts.CredentialFlow.ShouldSetup() {
		warning, err := opts.CredentialFlow.Setup(hostname, username, authToken, refreshableCredential != nil)
		if err != nil {
			return err
		}
		if warning != "" {
			fmt.Fprintf(opts.IO.ErrOut, "%s %s\n", cs.WarningIcon(), warning)
		}
	}

	// Record the final helper state for every sign-in. For non-interactive and SSH flows no setup runs, so this equals
	// the before value (gh observed the helper and changed nothing); for interactive HTTPS it reflects any setup. It is
	// derived from the flow's decisions rather than re-reading git configuration.
	authEvent.RecordHelperAfter(opts.CredentialFlow.ResultingHelperCategory())

	if keyToUpload != "" {
		uploaded, err := sshKeyUpload(httpClient, hostname, keyToUpload, keyTitle)
		if err != nil {
			return err
		}

		if uploaded {
			fmt.Fprintf(opts.IO.ErrOut, "%s Uploaded the SSH key to your GitHub account: %s\n", cs.SuccessIcon(), cs.Bold(keyToUpload))
		} else {
			fmt.Fprintf(opts.IO.ErrOut, "%s SSH key already existed on your GitHub account: %s\n", cs.SuccessIcon(), cs.Bold(keyToUpload))
		}
	}

	fmt.Fprintf(opts.IO.ErrOut, "%s Logged in as %s\n", cs.SuccessIcon(), cs.Bold(username))
	if userWasAlreadyLoggedIn {
		fmt.Fprintf(opts.IO.ErrOut, "%s You were already logged in to this account\n", cs.WarningIcon())
	}

	return nil
}

func scopesSentence(scopes []string) string {
	quoted := make([]string, len(scopes))
	for i, s := range scopes {
		quoted[i] = fmt.Sprintf("'%s'", s)
	}
	return strings.Join(quoted, ", ")
}

func sshKeyUpload(httpClient *http.Client, hostname, keyFile string, title string) (bool, error) {
	f, err := os.Open(keyFile)
	if err != nil {
		return false, err
	}
	defer f.Close()

	return add.SSHKeyUpload(httpClient, hostname, f, title)
}

// GetCurrentLogin returns the login of the user the token belongs to.
//
// The token is passed explicitly because this runs before the token is stored in config, so
// the transport has nothing to attach. The transport only sets Authorization when it is
// absent, so the header set here wins.
func GetCurrentLogin(httpClient *http.Client, hostname, authToken string) (string, error) {
	var result struct{ Viewer struct{ Login string } }

	// TODO(api-client-rollout)
	// This line of code is part of a mechanical roll out of the api client.
	// As a follow up, consider whether the api client can be injected to this call site, rather than constructed
	err := api.NewClientFromHTTP(httpClient).GraphQL(hostname, `query UserCurrent{viewer{login}}`, nil, &result,
		api.WithHeader("Authorization", "token "+authToken))
	if err != nil {
		return "", err
	}
	return result.Viewer.Login, nil
}
