package logout

import (
	"fmt"
	"slices"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/cmd/auth/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type LogoutOptions struct {
	IO       *iostreams.IOStreams
	Config   func() (config.Config, error)
	Prompter shared.Prompt
	Hostname string
	Username string
}

func NewCmdLogout(f *cmdutil.Factory, runF func(*LogoutOptions) error) *cobra.Command {
	opts := &LogoutOptions{
		IO:       f.IOStreams,
		Config:   f.Config,
		Prompter: f.Prompter,
	}

	cmd := &cobra.Command{
		Use:   "logout",
		Args:  cobra.ExactArgs(0),
		Short: "Log out of a GitHub user account",
		Long: heredoc.Doc(`
			Remove authentication for a GitHub user account.

			This command removes the authentication configuration for a user account
			either specified interactively or via the --hostname and --user flags.
		`),
		Example: heredoc.Doc(`
			# Select what host and user account to log out of via a prompt
			$ gh auth logout

			# Log out of specified user account on specified host
			$ gh auth logout --hostname enterprise.internal --user monalisa
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			// if (opts.Hostname == "" || opts.Username == "") && !opts.IO.CanPrompt() {
			// 	return cmdutil.FlagErrorf("--hostname and --user required when not running interactively")
			// }

			if runF != nil {
				return runF(opts)
			}

			return logoutRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Hostname, "hostname", "h", "", "The hostname of the GitHub instance to log out of")
	cmd.Flags().StringVarP(&opts.Username, "user", "u", "", "The user account to log out of")

	return cmd
}

func logoutRun(opts *LogoutOptions) error {
	hostname := opts.Hostname
	username := opts.Username

	cfg, err := opts.Config()
	if err != nil {
		return err
	}
	authCfg := cfg.Authentication()

	knownHosts := authCfg.Hosts()
	if len(knownHosts) == 0 {
		return fmt.Errorf("not logged in to any hosts")
	}

	if hostname != "" {
		if !slices.Contains(knownHosts, hostname) {
			return fmt.Errorf("not logged in to %s", hostname)
		}

		if username != "" {
			knownUsers, _ := cfg.Authentication().UsersForHost(hostname)
			if !slices.Contains(knownUsers, username) {
				return fmt.Errorf("not logged in as %s on %s", username, hostname)
			}
		}
	}

	type hostUser struct {
		host string
		user string
	}
	var candidates []hostUser

	for _, host := range knownHosts {
		if hostname != "" && host != hostname {
			continue
		}
		knownUsers, err := cfg.Authentication().UsersForHost(host)
		if err != nil {
			return err
		}
		for _, user := range knownUsers {
			if username != "" && user != username {
				continue
			}
			candidates = append(candidates, hostUser{host: host, user: user})
		}
	}

	if len(candidates) == 1 {
		hostname = candidates[0].host
		username = candidates[0].user
	} else if !opts.IO.CanPrompt() {
		return fmt.Errorf("unable to determine which user account to log out of, please specify %[1]s--hostname%[1]s and %[1]s--user%[1]s", "`")
	} else {
		prompts := make([]string, len(candidates))
		for i, c := range candidates {
			prompts[i] = fmt.Sprintf("%s (%s)", c.user, c.host)
		}
		selected, err := opts.Prompter.Select(
			"What account do you want to log out of?", "", prompts)
		if err != nil {
			return fmt.Errorf("could not prompt: %w", err)
		}
		hostname = candidates[selected].host
		username = candidates[selected].user
	}

	if src, writeable := shared.AuthTokenWriteable(authCfg, hostname); !writeable {
		fmt.Fprintf(opts.IO.ErrOut, "The value of the %s environment variable is being used for authentication.\n", src)
		fmt.Fprint(opts.IO.ErrOut, "To erase credentials stored in GitHub CLI, first clear the value from the environment.\n")
		return cmdutil.SilentError
	}

	// We can ignore the error here because a host must always have an active user
	preLogoutActiveUser, _ := authCfg.User(hostname)

	if err := authCfg.Logout(hostname, username); err != nil {
		return fmt.Errorf("failed to write config, authentication configuration not updated: %w", err)
	}

	isTTY := opts.IO.IsStdinTTY() && opts.IO.IsStdoutTTY()

	if isTTY {
		postLogoutActiveUser, _ := authCfg.User(hostname)
		hasSwitchedToNewUser := preLogoutActiveUser != postLogoutActiveUser &&
			postLogoutActiveUser != ""

		cs := opts.IO.ColorScheme()
		fmt.Fprintf(opts.IO.ErrOut, "%s Logged out of %s account '%s'\n",
			cs.SuccessIcon(), cs.Bold(hostname), username)

		if hasSwitchedToNewUser {
			fmt.Fprintf(opts.IO.ErrOut, "%s Switched account to '%s'\n",
				cs.SuccessIcon(), cs.Bold(postLogoutActiveUser))
		}
	}

	return nil
}
