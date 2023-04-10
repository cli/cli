package login

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/pkg/cmd/auth/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	ghAuth "github.com/cli/go-gh/v2/pkg/auth"
	"github.com/spf13/cobra"
)

type LoginOptions struct {
	IO         *iostreams.IOStreams
	Config     func() (config.Config, error)
	HttpClient func() (*http.Client, error)
	GitClient  *git.Client
	Prompter   shared.Prompt
	Browser    browser.Browser

	MainExecutable string

	Interactive bool

	Hostname        string
	Scopes          []string
	Token           string
	Web             bool
	GitProtocol     string
	InsecureStorage bool
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
		Short: "Authenticate with a GitHub host",
		Long: heredoc.Docf(`
			Authenticate with a GitHub host.

			The default authentication mode is a web-based browser flow. After completion, an
			authentication token will be stored internally.

			Alternatively, use %[1]s--with-token%[1]s to pass in a token on standard input.
			The minimum required scopes for the token are: "repo", "read:org".

			Alternatively, gh will use the authentication token found in environment variables.
			This method is most suitable for "headless" use of gh such as in automation. See
			%[1]sgh help environment%[1]s for more info.

			To use gh in GitHub Actions, add %[1]sGH_TOKEN: ${{ github.token }}%[1]s to "env".
		`, "`"),
		Example: heredoc.Doc(`
			# start interactive setup
			$ gh auth login

			# authenticate against github.com by reading the token from a file
			$ gh auth login --with-token < mytoken.txt

			# authenticate with a specific GitHub instance
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
				opts.Hostname, _ = ghAuth.DefaultHost()
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
	cmdutil.StringEnumFlag(cmd, &opts.GitProtocol, "git-protocol", "p", "", []string{"ssh", "https"}, "The protocol to use for git operations")

	// secure storage became the default on 2023/4/04; this flag is left as a no-op for backwards compatibility
	var secureStorage bool
	cmd.Flags().BoolVar(&secureStorage, "secure-storage", false, "Save authentication credentials in secure credential store")
	_ = cmd.Flags().MarkHidden("secure-storage")

	cmd.Flags().BoolVar(&opts.InsecureStorage, "insecure-storage", false, "Save authentication credentials in plain text instead of credential store")

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
		// Adding a user key ensures that a nonempty host section gets written to the config file.
		return authCfg.Login(hostname, "x-access-token", opts.Token, opts.GitProtocol, !opts.InsecureStorage)
	}

	existingToken, _ := authCfg.Token(hostname)
	if existingToken != "" && opts.Interactive {
		if err := shared.HasMinimumScopes(httpClient, hostname, existingToken); err == nil {
			keepGoing, err := opts.Prompter.Confirm(fmt.Sprintf("You're already logged into %s. Do you want to re-authenticate?", hostname), false)
			if err != nil {
				return err
			}
			if !keepGoing {
				return nil
			}
		}
	}

	return shared.Login(&shared.LoginOptions{
		IO:            opts.IO,
		Config:        authCfg,
		HTTPClient:    httpClient,
		Hostname:      hostname,
		Interactive:   opts.Interactive,
		Web:           opts.Web,
		Scopes:        opts.Scopes,
		Executable:    opts.MainExecutable,
		GitProtocol:   opts.GitProtocol,
		Prompter:      opts.Prompter,
		GitClient:     opts.GitClient,
		Browser:       opts.Browser,
		SecureStorage: !opts.InsecureStorage,
	})
}

func promptForHostname(opts *LoginOptions) (string, error) {
	options := []string{"GitHub.com", "GitHub Enterprise Server"}
	hostType, err := opts.Prompter.Select(
		"What account do you want to log into?",
		options[0],
		options)
	if err != nil {
		return "", err
	}

	isEnterprise := hostType == 1

	hostname := ghinstance.Default()
	if isEnterprise {
		hostname, err = opts.Prompter.InputHostname()
	}

	return hostname, err
}
