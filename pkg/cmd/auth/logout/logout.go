package logout

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/AlecAivazis/survey/v2"
	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/prompt"
	"github.com/spf13/cobra"
)

type LogoutOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	Config     func() (config.Config, error)

	Hostname string
}

func NewCmdLogout(f *cmdutil.Factory, runF func(*LogoutOptions) error) *cobra.Command {
	opts := &LogoutOptions{
		HttpClient: f.HttpClient,
		IO:         f.IOStreams,
		Config:     f.Config,
	}

	cmd := &cobra.Command{
		Use:   "logout",
		Args:  cobra.ExactArgs(0),
		Short: "Log out of a GitHub host",
		Long: heredoc.Doc(`Remove authentication for a GitHub host.

			This command removes the authentication configuration for a host either specified
			interactively or via --hostname.
		`),
		Example: heredoc.Doc(`
			$ gh auth logout
			# => select what host to log out of via a prompt

			$ gh auth logout --hostname enterprise.internal
			# => log out of specified host
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.Hostname == "" && !opts.IO.CanPrompt() {
				return &cmdutil.FlagError{Err: errors.New("--hostname required when not running interactively")}
			}

			if runF != nil {
				return runF(opts)
			}

			return logoutRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Hostname, "hostname", "h", "", "The hostname of the GitHub instance to log out of")

	return cmd
}

func logoutRun(opts *LogoutOptions) error {
	hostname := opts.Hostname

	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	candidates, err := cfg.Hosts()
	if err != nil {
		return fmt.Errorf("not logged in to any hosts")
	}

	if hostname == "" {
		if len(candidates) == 1 {
			hostname = candidates[0]
		} else {
			err = prompt.SurveyAskOne(&survey.Select{
				Message: "What account do you want to log out of?",
				Options: candidates,
			}, &hostname)

			if err != nil {
				return fmt.Errorf("could not prompt: %w", err)
			}
		}
	} else {
		var found bool
		for _, c := range candidates {
			if c == hostname {
				found = true
				break
			}
		}

		if !found {
			return fmt.Errorf("not logged into %s", hostname)
		}
	}

	if err := cfg.CheckWriteable(hostname, "oauth_token"); err != nil {
		return err
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	apiClient := api.NewClientFromHTTP(httpClient)

	username, err := api.CurrentLoginName(apiClient, hostname)
	if err != nil {
		// suppressing; the user is trying to delete this token and it might be bad.
		// we'll see if the username is in the config and fall back to that.
		username, _ = cfg.Get(hostname, "user")
	}

	usernameStr := ""
	if username != "" {
		usernameStr = fmt.Sprintf(" account '%s'", username)
	}

	if opts.IO.CanPrompt() {
		var keepGoing bool
		err := prompt.SurveyAskOne(&survey.Confirm{
			Message: fmt.Sprintf("Are you sure you want to log out of %s%s?", hostname, usernameStr),
			Default: true,
		}, &keepGoing)
		if err != nil {
			return fmt.Errorf("could not prompt: %w", err)
		}

		if !keepGoing {
			return nil
		}
	}

	cfg.UnsetHost(hostname)
	err = cfg.Write()
	if err != nil {
		return fmt.Errorf("failed to write config, authentication configuration not updated: %w", err)
	}

	isTTY := opts.IO.IsStdinTTY() && opts.IO.IsStdoutTTY()

	if isTTY {
		cs := opts.IO.ColorScheme()
		fmt.Fprintf(opts.IO.ErrOut, "%s Logged out of %s%s\n",
			cs.SuccessIcon(), cs.Bold(hostname), usernameStr)
	}

	return nil
}
