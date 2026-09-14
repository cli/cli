package refresh

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/authflow"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/cmd/auth/shared"
	"github.com/cli/cli/v2/pkg/cmd/auth/shared/gitcredentials"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/set"
	"github.com/spf13/cobra"
)

type RefreshOptions struct {
	IO              *iostreams.IOStreams
	Config          func() (gh.Config, error)
	PlainHttpClient func() (*http.Client, error)
	GitClient       *git.Client
	Prompter        shared.Prompt

	MainExecutable string

	Hostname     string
	Scopes       []string
	RemoveScopes []string
	ResetScopes  bool
	AuthFlow     func(*http.Client, *iostreams.IOStreams, string, []string, bool, bool, bool) (*authflow.AuthResult, error)

	Interactive     bool
	InsecureStorage bool
	ShortLived      bool
	Clipboard       *bool
}

func NewCmdRefresh(f *cmdutil.Factory, runF func(*RefreshOptions) error) *cobra.Command {
	opts := &RefreshOptions{
		IO:     f.IOStreams,
		Config: f.Config,
		AuthFlow: func(httpClient *http.Client, io *iostreams.IOStreams, hostname string, scopes []string, interactive, clipboard, requestRefreshToken bool) (*authflow.AuthResult, error) {
			return authflow.AuthFlow(httpClient, hostname, io, "", scopes, interactive, f.Browser, clipboard, requestRefreshToken)
		},
		PlainHttpClient: f.PlainHttpClient,
		GitClient:       f.GitClient,
		Prompter:        f.Prompter,
	}

	cmd := &cobra.Command{
		Use:   "refresh",
		Args:  cobra.ExactArgs(0),
		Short: "Refresh stored authentication credentials",
		Long: heredoc.Docf(`
			Expand or fix the permission scopes for stored credentials for active account.

			The %[1]s--scopes%[1]s flag accepts a comma separated list of scopes you want
			your gh credentials to have. If no scopes are provided, the command
			maintains previously added scopes.

			The %[1]s--remove-scopes%[1]s flag accepts a comma separated list of scopes you
			want to remove from your gh credentials. Scope removal is idempotent.
			The minimum set of scopes (%[1]srepo%[1]s, %[1]sread:org%[1]s, and %[1]sgist%[1]s) cannot be removed.

			The %[1]s--reset-scopes%[1]s flag resets the scopes for your gh credentials to
			the default set of scopes for your auth flow.

			If you have multiple accounts in %[1]sgh auth status%[1]s and want to refresh the credentials for an
			inactive account, you will have to use %[1]sgh auth switch%[1]s to that account first before using
			this command, and then switch back when you are done.

			Use %[1]s--short-lived%[1]s to prefer short-lived credentials that gh refreshes automatically.
			This is a preference, not a guarantee: whether short-lived tokens are issued depends on the
			host and the OAuth app configuration. A host without support issues a non-expiring token
			instead, and a host configured for them may issue short-lived tokens even without the flag.
			When using short-lived credentials for git operations, git 2.46 or newer is recommended so
			gh can mark the token as non-cacheable; with older git a credential caching helper may store
			and reuse an expired token.

			For more information on OAuth scopes, see
			<https://docs.github.com/en/developers/apps/building-oauth-apps/scopes-for-oauth-apps/>.
		`, "`"),
		Example: heredoc.Doc(`
			# Open a browser to add write:org and read:public_key scopes
			$ gh auth refresh --scopes write:org,read:public_key

			# Open a browser to ensure your authentication credentials have the correct minimum scopes
			$ gh auth refresh

			# Open a browser to idempotently remove the delete_repo scope
			$ gh auth refresh --remove-scopes delete_repo

			# Open a browser to re-authenticate with the default minimum scopes
			$ gh auth refresh --reset-scopes

			# Open a browser to re-authenticate and copy one-time OAuth code to clipboard
			$ gh auth refresh --clipboard
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Interactive = opts.IO.CanPrompt()

			if !opts.Interactive && opts.Hostname == "" {
				return cmdutil.FlagErrorf("--hostname required when not running interactively")
			}

			opts.MainExecutable = f.ExecutablePath
			if runF != nil {
				return runF(opts)
			}
			return refreshRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Hostname, "hostname", "h", "", "The GitHub host to use for authentication")
	cmd.Flags().StringSliceVarP(&opts.Scopes, "scopes", "s", nil, "Additional authentication scopes for gh to have")
	cmd.Flags().StringSliceVarP(&opts.RemoveScopes, "remove-scopes", "r", nil, "Authentication scopes to remove from gh")
	cmd.Flags().BoolVar(&opts.ResetScopes, "reset-scopes", false, "Reset authentication scopes to the default minimum set of scopes")
	cmdutil.NilBoolFlag(cmd, &opts.Clipboard, "clipboard", "c", "Copy one-time OAuth device code to clipboard")
	// secure storage became the default on 2023/4/04; this flag is left as a no-op for backwards compatibility
	var secureStorage bool
	cmd.Flags().BoolVar(&secureStorage, "secure-storage", false, "Save authentication credentials in secure credential store")
	_ = cmd.Flags().MarkHidden("secure-storage")

	cmd.Flags().BoolVarP(&opts.InsecureStorage, "insecure-storage", "", false, "Save authentication credentials in plain text instead of credential store")
	cmd.Flags().BoolVar(&opts.ShortLived, "short-lived", false, "Prefer short-lived credentials that gh refreshes automatically, if the host supports them")

	return cmd
}

func refreshRun(opts *RefreshOptions) error {
	plainHTTPClient, err := opts.PlainHttpClient()
	if err != nil {
		return err
	}

	cfg, err := opts.Config()
	if err != nil {
		return err
	}
	authCfg := cfg.Authentication()
	copyToClipboard := shared.ShouldCopyToClipboard(cfg, opts.Clipboard)

	candidates := authCfg.Hosts()
	if len(candidates) == 0 {
		return fmt.Errorf("not logged in to any hosts. Use 'gh auth login' to authenticate with a host")
	}

	hostname := opts.Hostname
	if hostname == "" {
		if len(candidates) == 1 {
			hostname = candidates[0]
		} else {
			selected, err := opts.Prompter.Select("What account do you want to refresh auth for?", "", candidates)
			if err != nil {
				return fmt.Errorf("could not prompt: %w", err)
			}
			hostname = candidates[selected]
		}
	} else if !slices.Contains(candidates, hostname) {
		return fmt.Errorf("not logged in to %s. use 'gh auth login' to authenticate with this host", hostname)
	}

	if src, writeable := shared.AuthTokenWriteable(authCfg, hostname); !writeable {
		fmt.Fprintf(opts.IO.ErrOut, "The value of the %s environment variable is being used for authentication.\n", src)
		fmt.Fprint(opts.IO.ErrOut, "To refresh credentials stored in GitHub CLI, first clear the value from the environment.\n")
		return cmdutil.SilentError
	}

	cs := opts.IO.ColorScheme()

	// Capture whether the stored credential is refreshable before it is replaced. Warn up front (before the credential
	// and browser prompts) so the user can abort rather than discover only afterwards that refreshing without
	// --short-lived downgrades it to a non-expiring token.
	wasRefreshable := authCfg.ActiveToken(hostname).IsRefreshable()
	if wasRefreshable && !opts.ShortLived {
		fmt.Fprintf(opts.IO.ErrOut, "%s Original token was short-lived; pass --short-lived to preserve it\n", cs.WarningIcon())
	}

	additionalScopes := set.NewStringSet()

	if !opts.ResetScopes {
		if oldToken := authCfg.ActiveToken(hostname).Token; oldToken != "" {
			if oldScopes, err := shared.GetScopes(plainHTTPClient, hostname, oldToken); err == nil {
				for s := range strings.SplitSeq(oldScopes, ",") {
					s = strings.TrimSpace(s)
					if s != "" {
						additionalScopes.Add(s)
					}
				}
			}
		}
	}

	credentialFlow := &shared.GitCredentialFlow{
		Prompter: opts.Prompter,
		HelperConfig: &gitcredentials.HelperConfig{
			SelfExecutablePath: opts.MainExecutable,
			GitClient:          opts.GitClient,
		},
		Updater: &gitcredentials.Updater{
			GitClient: opts.GitClient,
		},
	}
	gitProtocol := cfg.GitProtocol(hostname).Value
	if opts.Interactive && gitProtocol == "https" {
		if err := credentialFlow.Prompt(hostname); err != nil {
			return err
		}
		additionalScopes.AddValues(credentialFlow.Scopes())
	}

	additionalScopes.AddValues(opts.Scopes)

	additionalScopes.RemoveValues(opts.RemoveScopes)

	result, err := opts.AuthFlow(plainHTTPClient, opts.IO, hostname, additionalScopes.ToSlice(), opts.Interactive, copyToClipboard, opts.ShortLived)
	if err != nil {
		return err
	}
	activeUser, _ := authCfg.ActiveUser(hostname)
	if activeUser != "" && activeUser != result.Username {
		return fmt.Errorf("error refreshing credentials for %s, received credentials for %s, did you use the correct account in the browser?", activeUser, result.Username)
	}
	if result.Refreshable != nil {
		if _, err := authCfg.LoginRefreshable(hostname, result.Username, *result.Refreshable, "", !opts.InsecureStorage); err != nil {
			return err
		}
	} else {
		if _, err := authCfg.Login(hostname, result.Username, result.Token, "", !opts.InsecureStorage); err != nil {
			return err
		}
	}

	fmt.Fprintf(opts.IO.ErrOut, "%s Authentication complete.\n", cs.SuccessIcon())

	// Short-lived tokens are a preference, not a guarantee: the server decides based on its own and the OAuth app
	// configuration, so it may issue a refreshable token without being asked or a non-expiring one despite the request.
	switch {
	case result.Refreshable != nil && !opts.ShortLived:
		fmt.Fprintf(opts.IO.ErrOut, "%s Host issues short-lived refreshable token\n", cs.WarningIcon())
	case result.Refreshable != nil:
		fmt.Fprintf(opts.IO.ErrOut, "%s Received short-lived refreshable token\n", cs.SuccessIcon())
	case wasRefreshable && !opts.ShortLived:
		fmt.Fprintf(opts.IO.ErrOut, "%s Original token was short-lived; pass --short-lived to preserve it\n", cs.WarningIcon())
	case opts.ShortLived:
		fmt.Fprintf(opts.IO.ErrOut, "%s Host did not issue a short-lived refreshable token\n", cs.WarningIcon())
	}

	if credentialFlow.ShouldSetup() {
		// The active token was just minted by the auth flow above, so it cannot be expired here. We use
		// ActiveToken rather than the WithRefresh variant to avoid a needless refresh round-trip.
		username, _ := authCfg.ActiveUser(hostname)
		password := authCfg.ActiveToken(hostname).Token
		warning, err := credentialFlow.Setup(hostname, username, password, result.Refreshable != nil)
		if err != nil {
			return err
		}
		if warning != "" {
			fmt.Fprintf(opts.IO.ErrOut, "%s %s\n", cs.WarningIcon(), warning)
		}
	}

	return nil
}
