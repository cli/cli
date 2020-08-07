package refresh

import (
	"fmt"
	"os"

	"github.com/AlecAivazis/survey/v2"
	"github.com/MakeNowJust/heredoc"
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
}

func NewCmdRefresh(f *cmdutil.Factory, runF func(*RefreshOptions) error) *cobra.Command {
	opts := &RefreshOptions{
		IO:     f.IOStreams,
		Config: f.Config,
	}

	cmd := &cobra.Command{
		Use:   "refresh",
		Args:  cobra.ExactArgs(0),
		Short: "Request new scopes for a token",
		Long: heredoc.Doc(`Expand the permission scopes for a given host's token.

			This command allows you to add additional scopes to an existing authentication token via a web
			browser. This enables gh to access more of the GitHub API, which may be required as gh adds
			features or as you use the gh api command. 

			Unfortunately at this time there is no way to add scopes without a web browser's involvement
			due to how GitHub authentication works.

			The --hostname flag allows you to operate on a GitHub host other than github.com.

			The --scopes flag accepts a comma separated list of scopes you want to add to a token. If
			absent, this command ensures that a host's token has the default set of scopes required by gh.

			Note that if GITHUB_TOKEN is in the current environment, this command will not work.
		`),
		Example: heredoc.Doc(`
			$ gh auth refresh --scopes write:org,read:public_key
			# => open a browser to add write:org and read:public_key scopes for use with gh api

			$ gh auth refresh
			# => ensure that the required minimum scopes are enabled for a token and open a browser to add if not
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}

			return refreshRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Hostname, "hostname", "h", "", "The GitHub host to use for authentication")
	cmd.Flags().StringSliceVarP(&opts.Scopes, "scopes", "s", []string{}, "Additional scopes to add to a token")

	return cmd
}

func refreshRun(opts *RefreshOptions) error {
	if os.Getenv("GITHUB_TOKEN") != "" {
		return fmt.Errorf("GITHUB_TOKEN is present in your environment and is incompatible with this command. If you'd like to modify a personal access token, see https://github.com/settings/tokens")
	}

	isTTY := opts.IO.IsStdinTTY() && opts.IO.IsStdoutTTY()

	if !isTTY {
		return fmt.Errorf("not attached to a terminal; in headless environments, GITHUB_TOKEN is recommended")
	}

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

	return doAuthFlow(cfg, hostname, opts.Scopes)
}

var doAuthFlow = func(cfg config.Config, hostname string, scopes []string) error {
	_, err := config.AuthFlowWithConfig(cfg, hostname, "", scopes)
	return err
}
