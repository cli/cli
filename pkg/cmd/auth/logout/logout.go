package logout

import (
	"fmt"
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/cmd/auth/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type LogoutOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	Config     func() (config.Config, error)
	Prompter   shared.Prompt
	Hostname   string
}

func NewCmdLogout(f *cmdutil.Factory, runF func(*LogoutOptions) error) *cobra.Command {
	opts := &LogoutOptions{
		HttpClient: f.HttpClient,
		IO:         f.IOStreams,
		Config:     f.Config,
		Prompter:   f.Prompter,
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
				return cmdutil.FlagErrorf("--hostname required when not running interactively")
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
	authCfg := cfg.Authentication()

	candidates := authCfg.Hosts()
	if len(candidates) == 0 {
		return fmt.Errorf("not logged in to any hosts")
	}

	if hostname == "" {
		if len(candidates) == 1 {
			hostname = candidates[0]
		} else {
			selected, err := opts.Prompter.Select(
				"What account do you want to log out of?", "", candidates)
			if err != nil {
				return fmt.Errorf("could not prompt: %w", err)
			}
			hostname = candidates[selected]
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

	if src, writeable := shared.AuthTokenWriteable(authCfg, hostname); !writeable {
		fmt.Fprintf(opts.IO.ErrOut, "The value of the %s environment variable is being used for authentication.\n", src)
		fmt.Fprint(opts.IO.ErrOut, "To erase credentials stored in GitHub CLI, first clear the value from the environment.\n")
		return cmdutil.SilentError
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
		username, _ = authCfg.User(hostname)
	}

	usernameStr := ""
	if username != "" {
		usernameStr = fmt.Sprintf(" account '%s'", username)
	}

	if err := authCfg.Logout(hostname); err != nil {
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
