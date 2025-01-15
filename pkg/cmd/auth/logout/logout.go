package logout

import (
	"errors"
	"fmt"
	"slices"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/cmd/auth/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type LogoutOptions struct {
	IO       *iostreams.IOStreams
	Config   func() (gh.Config, error)
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
		Short: "Log out of a GitHub account",
		Long: heredoc.Doc(`
			Remove authentication for a GitHub account.

			This command removes the stored authentication configuration
			for an account. The authentication configuration is only
			removed locally.

			This command does not invalidate authentication tokens.
		`),
		Example: heredoc.Doc(`
			# Select what host and account to log out of via a prompt
			$ gh auth logout

			# Log out of a specific host and specific account
			$ gh auth logout --hostname enterprise.internal --user monalisa
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}

			return logoutRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Hostname, "hostname", "h", "", "The hostname of the GitHub instance to log out of")
	cmd.Flags().StringVarP(&opts.Username, "user", "u", "", "The account to log out of")

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
			knownUsers := cfg.Authentication().UsersForHost(hostname)
			if !slices.Contains(knownUsers, username) {
				return fmt.Errorf("not logged in to %s account %s", hostname, username)
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
		knownUsers := cfg.Authentication().UsersForHost(host)
		for _, user := range knownUsers {
			if username != "" && user != username {
				continue
			}
			candidates = append(candidates, hostUser{host: host, user: user})
		}
	}

	if len(candidates) == 0 {
		return errors.New("no accounts matched that criteria")
	} else if len(candidates) == 1 {
		hostname = candidates[0].host
		username = candidates[0].user
	} else if !opts.IO.CanPrompt() {
		return errors.New("unable to determine which account to log out of, please specify `--hostname` and `--user`")
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
	preLogoutActiveUser, _ := authCfg.ActiveUser(hostname)

	if err := authCfg.Logout(hostname, username); err != nil {
		return err
	}

	postLogoutActiveUser, _ := authCfg.ActiveUser(hostname)
	hasSwitchedToNewUser := preLogoutActiveUser != postLogoutActiveUser &&
		postLogoutActiveUser != ""

	cs := opts.IO.ColorScheme()
	fmt.Fprintf(opts.IO.ErrOut, "%s Logged out of %s account %s\n",
		cs.SuccessIcon(), hostname, cs.Bold(username))

	if hasSwitchedToNewUser {
		fmt.Fprintf(opts.IO.ErrOut, "%s Switched active account for %s to %s\n",
			cs.SuccessIcon(), hostname, cs.Bold(postLogoutActiveUser))
	}

	return nil
}
