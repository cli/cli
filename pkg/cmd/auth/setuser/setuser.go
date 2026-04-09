package authsetuser

import (
	"fmt"
	"slices"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	ghauth "github.com/cli/go-gh/v2/pkg/auth"
	"github.com/spf13/cobra"
)

// SetUserOptions holds the options for the set-user command.
type SetUserOptions struct {
	IO       *iostreams.IOStreams
	Config   func() (gh.Config, error)
	Hostname string
	Owner    string
	Username string
}

// NewCmdSetUser creates the `gh auth set-user` command.
func NewCmdSetUser(f *cmdutil.Factory, runF func(*SetUserOptions) error) *cobra.Command {
	opts := SetUserOptions{
		IO:     f.IOStreams,
		Config: f.Config,
	}

	cmd := &cobra.Command{
		Use:   "set-user <username>",
		Args:  cobra.ExactArgs(1),
		Short: "Map a GitHub owner to an authenticated user",
		Long: heredoc.Docf(`
			Map a GitHub owner (user or org) to an authenticated gh account.

			When gh runs a command inside a repository whose owner matches the
			given %[1]s--owner%[1]s value, it will automatically use the token for
			%[1]s<username>%[1]s instead of the globally active account.

			This allows working with repositories owned by different GitHub accounts
			without running %[1]sgh auth switch%[1]s between them.

			Run %[1]sgh auth status%[1]s to see available authenticated accounts.
		`, "`"),
		Example: heredoc.Doc(`
			# Use your work account for all repos owned by your work org
			$ gh auth set-user --owner xgdevops lukemckechnie

			# Use your personal account for your own repos
			$ gh auth set-user --owner galamdring galamdring

			# Set a mapping for a specific GitHub Enterprise host
			$ gh auth set-user --owner myorg --hostname enterprise.internal myuser
		`),
		RunE: func(c *cobra.Command, args []string) error {
			opts.Username = args[0]
			if runF != nil {
				return runF(&opts)
			}
			return setUserRun(&opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Owner, "owner", "o", "", "The GitHub owner (user or org) to map (required)")
	cmd.Flags().StringVarP(&opts.Hostname, "hostname", "h", "", "The hostname of the GitHub instance (default: github.com)")
	_ = cmd.MarkFlagRequired("owner")

	return cmd
}

func setUserRun(opts *SetUserOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}
	authCfg := cfg.Authentication()

	hostname := opts.Hostname
	if hostname == "" {
		hostname, _ = authCfg.DefaultHost()
	}
	hostname = ghauth.NormalizeHostname(hostname)

	// Validate that the hostname is known.
	if !slices.Contains(authCfg.Hosts(), hostname) {
		return fmt.Errorf("not logged in to %s", hostname)
	}

	// Validate that the username is a known authenticated user on this host.
	if !slices.Contains(authCfg.UsersForHost(hostname), opts.Username) {
		return fmt.Errorf("not logged in to %s as %s", hostname, opts.Username)
	}

	if err := authCfg.SetOwnerUser(hostname, opts.Owner, opts.Username); err != nil {
		return fmt.Errorf("failed to save owner mapping: %w", err)
	}

	cs := opts.IO.ColorScheme()
	fmt.Fprintf(opts.IO.ErrOut, "%s Mapped owner %s to user %s on %s\n",
		cs.SuccessIcon(), cs.Bold(opts.Owner), cs.Bold(opts.Username), hostname)

	return nil
}
