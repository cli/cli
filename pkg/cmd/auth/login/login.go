package login

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/internal/ghinstance"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/prompt"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

type LoginOptions struct {
	IO     *iostreams.IOStreams
	Config func() (config.Config, error)

	Hostname     string
	Token        string
	OnlyValidate bool
}

func NewCmdLogin(f *cmdutil.Factory, runF func(*LoginOptions) error) *cobra.Command {
	opts := &LoginOptions{
		IO:     f.IOStreams,
		Config: f.Config,
	}

	cmd := &cobra.Command{
		Use:   "login",
		Args:  cobra.ExactArgs(0),
		Short: "Authenticate with a GitHub host",
		Long: heredoc.Doc(`Authenticate with a GitHub host.

			This interactive command initializes your authentication state either by helping you log into
			GitHub via browser-based OAuth or by accepting a Personal Access Token.

			The interactivity can be avoided by specifying --with-token and passing a token on STDIN.
		`),
		Example: heredoc.Doc(`
			$ gh auth login
			# => do an interactive setup

			$ gh auth login --with-token < mytoken.txt
			# => read token from mytoken.txt and authenticate against github.com

			$ gh auth login --hostname enterprise.internal --with-token < mytoken.txt
			# => read token from mytoken.txt and authenticate against a GitHub Enterprise instance
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			isTTY := opts.IO.IsStdinTTY()

			// TODO support other ways of naming
			ghToken := os.Getenv("GITHUB_TOKEN")

			if !isTTY && (!cmd.Flags().Changed("with-token") && ghToken == "") {
				return &cmdutil.FlagError{Err: errors.New("no terminal detected; please use '--with-token' or set GITHUB_TOKEN")}
			}

			wt, _ := cmd.Flags().GetBool("with-token")
			if wt {
				defer opts.IO.In.Close()
				token, err := ioutil.ReadAll(opts.IO.In)
				if err != nil {
					return &cmdutil.FlagError{Err: fmt.Errorf("failed to read token from STDIN: %w", err)}
				}

				opts.Token = strings.TrimSpace(string(token))
			} else if ghToken != "" {
				opts.OnlyValidate = true
				opts.Token = ghToken
			}

			if opts.Token != "" {
				// Assume non-interactive if a token is specified
				if opts.Hostname == "" {
					opts.Hostname = ghinstance.Default()
				}
			}

			if runF != nil {
				return runF(opts)
			}

			return loginRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Hostname, "hostname", "h", "", "The hostname of the GitHub instance to authenticate with")
	cmd.Flags().Bool("with-token", false, "Read token from standard input")

	return cmd
}

func loginRun(opts *LoginOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	if opts.Token != "" {
		// I chose to not error on existing host here; my thinking is that for --with-token the user
		// probably doesn't care if a token is overwritten since they have a token in hand they
		// explicitly want to use.
		if opts.Hostname == "" {
			return errors.New("empty hostname would leak oauth_token")
		}

		err := cfg.Set(opts.Hostname, "oauth_token", opts.Token)
		if err != nil {
			return err
		}

		err = validateHostCfg(opts.Hostname, cfg)
		if err != nil {
			return err
		}

		if opts.OnlyValidate {
			return nil
		}

		return cfg.Write()
	}

	// TODO consider explicitly telling survey what io to use since it's implicit right now

	hostname := opts.Hostname

	if hostname == "" {
		var hostType int
		err := prompt.SurveyAskOne(&survey.Select{
			Message: "What account do you want to log into?",
			Options: []string{
				"GitHub.com",
				"GitHub Enterprise",
			},
		}, &hostType)

		if err != nil {
			return fmt.Errorf("could not prompt: %w", err)
		}

		isEnterprise := hostType == 1

		hostname = ghinstance.Default()
		if isEnterprise {
			err := prompt.SurveyAskOne(&survey.Input{
				Message: "GHE hostname:",
			}, &hostname, survey.WithValidator(survey.Required))
			if err != nil {
				return fmt.Errorf("could not prompt: %w", err)
			}
		}
	}

	fmt.Fprintf(opts.IO.ErrOut, "- Logging into %s\n", hostname)

	existingToken, _ := cfg.Get(hostname, "oauth_token")

	if existingToken != "" {
		err := validateHostCfg(hostname, cfg)
		if err == nil {
			apiClient, err := clientFromCfg(hostname, cfg)
			if err != nil {
				return err
			}

			username, err := api.CurrentLoginName(apiClient, hostname)
			if err != nil {
				return fmt.Errorf("error using api: %w", err)
			}
			var keepGoing bool
			err = prompt.SurveyAskOne(&survey.Confirm{
				Message: fmt.Sprintf(
					"You're already logged into %s as %s. Do you want to re-authenticate?",
					hostname,
					username),
				Default: false,
			}, &keepGoing)
			if err != nil {
				return fmt.Errorf("could not prompt: %w", err)
			}

			if !keepGoing {
				return nil
			}
		}
	}

	var authMode int
	err = prompt.SurveyAskOne(&survey.Select{
		Message: "How would you like to authenticate?",
		Options: []string{
			"Login with a web browser",
			"Paste an authentication token",
		},
	}, &authMode)
	if err != nil {
		return fmt.Errorf("could not prompt: %w", err)
	}

	if authMode == 0 {
		_, err := config.AuthFlowWithConfig(cfg, hostname, "")
		if err != nil {
			return fmt.Errorf("failed to authenticate via web browser: %w", err)
		}
	} else {
		fmt.Fprintln(opts.IO.ErrOut)
		fmt.Fprintln(opts.IO.ErrOut, heredoc.Doc(`
				Tip: you can generate a Personal Access Token here https://github.com/settings/tokens
				The minimum required scopes are 'repo' and 'read:org'.`))
		var token string
		err := prompt.SurveyAskOne(&survey.Password{
			Message: "Paste your authentication token:",
		}, &token, survey.WithValidator(survey.Required))
		if err != nil {
			return fmt.Errorf("could not prompt: %w", err)
		}

		if hostname == "" {
			return errors.New("empty hostname would leak oauth_token")
		}

		err = cfg.Set(hostname, "oauth_token", token)
		if err != nil {
			return err
		}

		err = validateHostCfg(hostname, cfg)
		if err != nil {
			return err
		}
	}

	var gitProtocol string
	err = prompt.SurveyAskOne(&survey.Select{
		Message: "Choose default git protocol",
		Options: []string{
			"HTTPS",
			"SSH",
		},
	}, &gitProtocol)
	if err != nil {
		return fmt.Errorf("could not prompt: %w", err)
	}

	gitProtocol = strings.ToLower(gitProtocol)

	fmt.Fprintf(opts.IO.ErrOut, "- gh config set -h%s git_protocol %s\n", hostname, gitProtocol)
	err = cfg.Set(hostname, "git_protocol", gitProtocol)
	if err != nil {
		return err
	}

	fmt.Fprintf(opts.IO.ErrOut, "%s Configured git protocol\n", utils.GreenCheck())

	apiClient, err := clientFromCfg(hostname, cfg)
	if err != nil {
		return err
	}

	username, err := api.CurrentLoginName(apiClient, hostname)
	if err != nil {
		return fmt.Errorf("error using api: %w", err)
	}

	err = cfg.Set(hostname, "user", username)
	if err != nil {
		return err
	}

	err = cfg.Write()
	if err != nil {
		return err
	}

	fmt.Fprintf(opts.IO.ErrOut, "%s Logged in as %s\n", utils.GreenCheck(), utils.Bold(username))

	return nil
}
