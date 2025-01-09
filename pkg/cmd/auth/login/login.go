package login

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/pkg/cmd/auth/shared"
	"github.com/cli/cli/v2/pkg/cmd/auth/shared/gitcredentials"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	ghauth "github.com/cli/go-gh/v2/pkg/auth"
	"github.com/spf13/cobra"
)

type LoginOptions struct {
	IO         *iostreams.IOStreams
	Config     func() (gh.Config, error)
	HttpClient func() (*http.Client, error)
	GitClient  *git.Client
	Prompter   shared.Prompt
	Browser    browser.Browser

	MainExecutable string

	Interactive bool

	Hostname         string
	Scopes           []string
	Token            string
	Web              bool
	GitProtocol      string
	InsecureStorage  bool
	SkipSSHKeyPrompt bool
}

func NewCmdLogin(f *cmdutil.Factory, runF func(*LoginOptions) error) *cobra.Command {
	opts := &LoginOptions{
		IO:         f.IOStreams,
		Config:     f.Config,
		HttpClient: f.HttpClient,
		GitClient:  f.GitClient,
		Prompter:   f.Prompter,
		Browser:    f.Browser,
	}

	var tokenStdin bool

	cmd := &cobra.Command{
		Use:   "login",
		Args:  cobra.ExactArgs(0),
		Short: "Log in to a GitHub account",
		Long: heredoc.Docf(`
			Authenticate with a GitHub host.

			The default hostname is %[1]sgithub.com%[1]s. This can be overridden using the %[1]s--hostname%[1]s
			flag.

			The default authentication mode is a web-based browser flow. After completion, an
			authentication token will be stored securely in the system credential store.
			If a credential store is not found or there is an issue using it gh will fallback
			to writing the token to a plain text file. See %[1]sgh auth status%[1]s for its
			stored location.

			Alternatively, use %[1]s--with-token%[1]s to pass in a personal access token (classic) on standard input.
			The minimum required scopes for the token are: %[1]srepo%[1]s, %[1]sread:org%[1]s, and %[1]sgist%[1]s.
			Take care when passing a fine-grained personal access token to %[1]s--with-token%[1]s
			as the inherent scoping to certain resources may cause confusing behaviour when interacting with other
			resources. Favour setting %[1]sGH_TOKEN$%[1]s for fine-grained personal access token usage. 

			Alternatively, gh will use the authentication token found in environment variables.
			This method is most suitable for "headless" use of gh such as in automation. See
			%[1]sgh help environment%[1]s for more info.

			To use gh in GitHub Actions, add %[1]sGH_TOKEN: ${{ github.token }}%[1]s to %[1]senv%[1]s.

			The git protocol to use for git operations on this host can be set with %[1]s--git-protocol%[1]s,
			or during the interactive prompting. Although login is for a single account on a host, setting
			the git protocol will take effect for all users on the host.

			Specifying %[1]sssh%[1]s for the git protocol will detect existing SSH keys to upload,
			prompting to create and upload a new key if one is not found. This can be skipped with
			%[1]s--skip-ssh-key%[1]s flag.

			For more information on OAuth scopes, <https://docs.github.com/en/developers/apps/building-oauth-apps/scopes-for-oauth-apps/>.
		`, "`"),
		Example: heredoc.Doc(`
			# Start interactive setup
			$ gh auth login

			# Authenticate against github.com by reading the token from a file
			$ gh auth login --with-token < mytoken.txt

			# Authenticate with specific host
			$ gh auth login --hostname enterprise.internal
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if tokenStdin && opts.Web {
				return cmdutil.FlagErrorf("specify only one of `--web` or `--with-token`")
			}
			if tokenStdin && len(opts.Scopes) > 0 {
				return cmdutil.FlagErrorf("specify only one of `--scopes` or `--with-token`")
			}

			if tokenStdin {
				defer opts.IO.In.Close()
				token, err := io.ReadAll(opts.IO.In)
				if err != nil {
					return fmt.Errorf("failed to read token from standard input: %w", err)
				}
				opts.Token = strings.TrimSpace(string(token))
			}

			if opts.IO.CanPrompt() && opts.Token == "" {
				opts.Interactive = true
			}

			if cmd.Flags().Changed("hostname") {
				if err := ghinstance.HostnameValidator(opts.Hostname); err != nil {
					return cmdutil.FlagErrorf("error parsing hostname: %w", err)
				}
			}

			if opts.Hostname == "" && (!opts.Interactive || opts.Web) {
				opts.Hostname, _ = ghauth.DefaultHost()
			}

			opts.MainExecutable = f.Executable()
			if runF != nil {
				return runF(opts)
			}

			return loginRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Hostname, "hostname", "h", "", "The hostname of the GitHub instance to authenticate with")
	cmd.Flags().StringSliceVarP(&opts.Scopes, "scopes", "s", nil, "Additional authentication scopes to request")
	cmd.Flags().BoolVar(&tokenStdin, "with-token", false, "Read token from standard input")
	cmd.Flags().BoolVarP(&opts.Web, "web", "w", false, "Open a browser to authenticate")
	cmdutil.StringEnumFlag(cmd, &opts.GitProtocol, "git-protocol", "p", "", []string{"ssh", "https"}, "The protocol to use for git operations on this host")

	// secure storage became the default on 2023/4/04; this flag is left as a no-op for backwards compatibility
	var secureStorage bool
	cmd.Flags().BoolVar(&secureStorage, "secure-storage", false, "Save authentication credentials in secure credential store")
	_ = cmd.Flags().MarkHidden("secure-storage")

	cmd.Flags().BoolVar(&opts.InsecureStorage, "insecure-storage", false, "Save authentication credentials in plain text instead of credential store")
	cmd.Flags().BoolVar(&opts.SkipSSHKeyPrompt, "skip-ssh-key", false, "Skip generate/upload SSH key prompt")

	return cmd
}

func loginRun(opts *LoginOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}
	authCfg := cfg.Authentication()

	hostname := opts.Hostname
	if opts.Interactive && hostname == "" {
		var err error
		hostname, err = promptForHostname(opts)
		if err != nil {
			return err
		}
	}

	// The go-gh Config object currently does not support case-insensitive lookups for host names,
	// so normalize the host name case here before performing any lookups with it or persisting it.
	// https://github.com/cli/go-gh/pull/105
	hostname = strings.ToLower(hostname)

	if src, writeable := shared.AuthTokenWriteable(authCfg, hostname); !writeable {
		fmt.Fprintf(opts.IO.ErrOut, "The value of the %s environment variable is being used for authentication.\n", src)
		fmt.Fprint(opts.IO.ErrOut, "To have GitHub CLI store credentials instead, first clear the value from the environment.\n")
		return cmdutil.SilentError
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	if opts.Token != "" {
		if err := shared.HasMinimumScopes(httpClient, hostname, opts.Token); err != nil {
			return fmt.Errorf("error validating token: %w", err)
		}
		username, err := shared.GetCurrentLogin(httpClient, hostname, opts.Token)
		if err != nil {
			return fmt.Errorf("error retrieving current user: %w", err)
		}

		// Adding a user key ensures that a nonempty host section gets written to the config file.
		_, loginErr := authCfg.Login(hostname, username, opts.Token, opts.GitProtocol, !opts.InsecureStorage)
		return loginErr
	}

	return shared.Login(&shared.LoginOptions{
		IO:          opts.IO,
		Config:      authCfg,
		HTTPClient:  httpClient,
		Hostname:    hostname,
		Interactive: opts.Interactive,
		Web:         opts.Web,
		Scopes:      opts.Scopes,
		GitProtocol: opts.GitProtocol,
		Prompter:    opts.Prompter,
		Browser:     opts.Browser,
		CredentialFlow: &shared.GitCredentialFlow{
			Prompter: opts.Prompter,
			HelperConfig: &gitcredentials.HelperConfig{
				SelfExecutablePath: opts.MainExecutable,
				GitClient:          opts.GitClient,
			},
			Updater: &gitcredentials.Updater{
				GitClient: opts.GitClient,
			},
		},
		SecureStorage:    !opts.InsecureStorage,
		SkipSSHKeyPrompt: opts.SkipSSHKeyPrompt,
	})
}

func promptForHostname(opts *LoginOptions) (string, error) {
	options := []string{"GitHub.com", "Other"}
	hostType, err := opts.Prompter.Select(
		"Where do you use GitHub?",
		options[0],
		options)
	if err != nil {
		return "", err
	}

	isGitHubDotCom := hostType == 0
	if isGitHubDotCom {
		return ghinstance.Default(), nil
	}

	return opts.Prompter.InputHostname()
}
