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
	// TODO: opts.Username likeley needed in the future
	username := ""

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
			candidates = append(candidates, hostUser{host: host, user: user})
		}
	}

	// We can ignore the error here because a host must always have an active user
	preLogoutActiveUser, _ := authCfg.User(hostname)

	if len(candidates) == 1 {
		hostname = candidates[0].host
		username = candidates[0].user
	} else if !opts.IO.CanPrompt() {
		username = preLogoutActiveUser
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

	if err := authCfg.Logout(hostname, username); err != nil {
		return fmt.Errorf("failed to write config, authentication configuration not updated: %w", err)
	}

	postLogoutActiveUser, _ := authCfg.User(hostname)
	hasSwitchedToNewUser := preLogoutActiveUser != postLogoutActiveUser &&
		postLogoutActiveUser != ""

	isTTY := opts.IO.IsStdinTTY() && opts.IO.IsStdoutTTY()

	if isTTY {
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
