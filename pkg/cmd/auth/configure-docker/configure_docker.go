package configure_docker

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
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
	dockerconfig "github.com/docker/cli/cli/config"
	"github.com/spf13/cobra"
)

type ConfigureDockerOptions struct {
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

func NewCmdConfigureDocker(f *cmdutil.Factory, runF func(*ConfigureDockerOptions) error) *cobra.Command {
	opts := &ConfigureDockerOptions{
		IO:         f.IOStreams,
		Config:     f.Config,
		HttpClient: f.HttpClient,
		GitClient:  f.GitClient,
		Prompter:   f.Prompter,
		Browser:    f.Browser,
	}

	var tokenStdin bool

	cmd := &cobra.Command{
		Use:   "configure-docker",
		Args:  cobra.ExactArgs(0),
		Short: "Configure a Docker credential helper.",
		Long: heredoc.Docf(`
			Configure a Docker credential helper.
		`, "`"),
		Example: heredoc.Doc(`
			# configure the Docker credential helper.
			$ gh auth configure-docker

			# authenticate against github.com by reading the token from a file
			$ gh auth configure-docker --with-token < mytoken.txt

			# authenticate with a specific GitHub instance
			$ gh auth configure-docker --hostname enterprise.internal
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

			return configureDockerRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Hostname, "hostname", "h", "", "The hostname of the GitHub instance to authenticate with")
	cmd.Flags().StringSliceVarP(&opts.Scopes, "scopes", "s", []string{"write:packages"}, "Additional authentication scopes to request")
	cmd.Flags().BoolVar(&tokenStdin, "with-token", false, "Read token from standard input")
	cmd.Flags().BoolVarP(&opts.Web, "web", "w", false, "Open a browser to authenticate")
	cmd.Flags().BoolVar(&opts.InsecureStorage, "insecure-storage", false, "Save authentication credentials in plain text instead of credential store")
	return cmd
}

func configureDockerRun(opts *ConfigureDockerOptions) error {
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
		// Adding a user key ensures that a nonempty host section gets written to the config file.
		if err := authCfg.Login(hostname, "x-access-token", opts.Token, opts.GitProtocol, !opts.InsecureStorage); err != nil {
			return err
		}
	} else {
		existingToken, _ := authCfg.Token(hostname)
		if existingToken != "" && opts.Interactive {
			if err := shared.HasMinimumScopes(httpClient, hostname, existingToken, opts.Scopes...); err == nil {
				keepGoing, err := opts.Prompter.Confirm(fmt.Sprintf("You're already logged into %s. Do you want to re-authenticate?", hostname), false)
				if err != nil {
					return err
				}
				if !keepGoing {
					return nil
				}
			}
		}
		if err := shared.Login(&shared.LoginOptions{
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
		}); err != nil {
			return err
		}
	}

	// With creds in place, we can now configure the Docker credential helper.

	// Create a symlink at the executable's current path, which is assumed to be on $PATH.
	ex, err := os.Executable()
	if err != nil {
		return fmt.Errorf("getting executable location: %w", err)
	}
	// If chainctl is not on PATH, then symlinking docker-credential-cgr
	// won't work when docker tries to execute it, so fail instead.
	onPath := false
	for _, pathEl := range strings.Split(os.Getenv("PATH"), string(os.PathListSeparator)) {
		if filepath.Dir(ex) == filepath.Clean(pathEl) {
			onPath = true
			break
		}
	}
	if !onPath {
		return errors.New("gh is not on PATH")
	}
	if err := os.Symlink(ex, filepath.Join(filepath.Dir(ex), "docker-credential-gh")); err != nil && !os.IsExist(err) {
		return fmt.Errorf("creating symlink: %w", err)
	}

	// Update the user's docker config to configure the cred helper for the registry endpoint.
	cf, err := dockerconfig.Load(os.Getenv("DOCKER_CONFIG"))
	if err != nil {
		return err
	}
	if cf.CredentialHelpers == nil {
		cf.CredentialHelpers = map[string]string{}
	}
	registry := "ghcr.io" // TODO: use configured hostname
	if cf.CredentialHelpers[registry] != "gh" {
		cf.CredentialHelpers[registry] = "gh"
		fmt.Fprintf(opts.IO.ErrOut, "Updated auth config for %s\n", registry)
	} else {
		fmt.Fprintf(opts.IO.ErrOut, "Credential helper already configured for %s\n", registry)
	}
	if _, ok := cf.AuthConfigs[registry]; ok {
		delete(cf.AuthConfigs, registry)
		fmt.Fprintf(opts.IO.ErrOut, "Removed existing token for %s\n", registry)
	}
	if err := cf.Save(); err != nil {
		return fmt.Errorf("writing docker config: %w", err)
	}

	return nil
}

func promptForHostname(opts *ConfigureDockerOptions) (string, error) {
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
