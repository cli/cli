package refresh

import (
	"errors"
	"fmt"

	"github.com/AlecAivazis/survey/v2"
	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/internal/authflow"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/prompt"
	"github.com/spf13/cobra"
)

type RefreshOptions struct {
	IO     *iostreams.IOStreams
	Config func() (config.Config, error)

	Hostname string
	Scopes   []string
	AuthFlow func(config.Config, *iostreams.IOStreams, string, []string) error
}

func NewCmdRefresh(f *cmdutil.Factory, runF func(*RefreshOptions) error) *cobra.Command {
	opts := &RefreshOptions{
		IO:     f.IOStreams,
		Config: f.Config,
		AuthFlow: func(cfg config.Config, io *iostreams.IOStreams, hostname string, scopes []string) error {
			_, err := authflow.AuthFlowWithConfig(cfg, io, hostname, "", scopes)
			return err
		},
	}

	cmd := &cobra.Command{
		Use:   "refresh",
		Args:  cobra.ExactArgs(0),
		Short: "Refresh stored authentication credentials",
		Long: heredoc.Doc(`Expand or fix the permission scopes for stored credentials

			The --scopes flag accepts a comma separated list of scopes you want your gh credentials to have. If
			absent, this command ensures that gh has access to a minimum set of scopes.
		`),
		Example: heredoc.Doc(`
			$ gh auth refresh --scopes write:org,read:public_key
			# => open a browser to add write:org and read:public_key scopes for use with gh api

			$ gh auth refresh
			# => open a browser to ensure your authentication credentials have the correct minimum scopes
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			isTTY := opts.IO.IsStdinTTY() && opts.IO.IsStdoutTTY()

			if !isTTY {
				return fmt.Errorf("not attached to a terminal; in headless environments, GITHUB_TOKEN is recommended")
			}

			if opts.Hostname == "" && !opts.IO.CanPrompt() {
				// here, we know we are attached to a TTY but prompts are disabled
				return &cmdutil.FlagError{Err: errors.New("--hostname required when not running interactively")}
			}

			if runF != nil {
				return runF(opts)
			}

			return refreshRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Hostname, "hostname", "h", "", "The GitHub host to use for authentication")
	cmd.Flags().StringSliceVarP(&opts.Scopes, "scopes", "s", nil, "Additional authentication scopes for gh to have")

	return cmd
}

func refreshRun(opts *RefreshOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	candidates, err := cfg.Hosts()
	if err != nil {
		return fmt.Errorf("not logged in to any hosts. Use 'gh auth login' to authenticate with a host")
	}

	hostname := opts.Hostname
	if hostname == "" {
		if len(candidates) == 1 {
			hostname = candidates[0]
		} else {
			err := prompt.SurveyAskOne(&survey.Select{
				Message: "What account do you want to refresh auth for?",
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
			return fmt.Errorf("not logged in to %s. use 'gh auth login' to authenticate with this host", hostname)
		}
	}

	if err := cfg.CheckWriteable(hostname, "oauth_token"); err != nil {
		return err
	}

	return opts.AuthFlow(cfg, opts.IO, hostname, opts.Scopes)
}
