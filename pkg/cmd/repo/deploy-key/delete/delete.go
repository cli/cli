package delete

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/AlecAivazis/survey/v2"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/prompt"
	"github.com/spf13/cobra"
)

type DeleteOptions struct {
	IO         *iostreams.IOStreams
	Config     func() (config.Config, error)
	HTTPClient func() (*http.Client, error)
	BaseRepo   func() (ghrepo.Interface, error)

	ID          string
	SkipConfirm bool
}

func NewCmdDelete(f *cmdutil.Factory, runF func(*DeleteOptions) error) *cobra.Command {
	opts := &DeleteOptions{
		HTTPClient: f.HttpClient,
		Config:     f.Config,
		IO:         f.IOStreams,
		BaseRepo:   f.BaseRepo,
	}

	cmd := &cobra.Command{
		Use:   "delete [<deploy-key-id>]",
		Short: "Delete a deploy key from your GitHub repository",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmdutil.FlagErrorf("deploy key id missing")
			}
			opts.ID = args[0]

			if runF != nil {
				return runF(opts)
			}
			return deleteRun(opts)
		},
	}
	cmd.Flags().BoolVarP(&opts.SkipConfirm, "skip-confirmation", "s", false, "dangerously skip confirmation to delete deploy key")

	return cmd
}

func deleteRun(opts *DeleteOptions) error {
	httpClient, err := opts.HTTPClient()
	if err != nil {
		return err
	}

	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	hostname, err := cfg.DefaultHost()
	if err != nil {
		return err
	}

	repo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	deployKey, err := deployKeyByID(httpClient, hostname, opts.ID, repo)
	if err != nil {
		if errors.Is(err, scopesError) {
			cs := opts.IO.ColorScheme()
			fmt.Fprint(opts.IO.ErrOut, "Error: insufficient OAuth scopes to delete deploy key\n")
			fmt.Fprintf(opts.IO.ErrOut, "Run the following to grant scopes: %s\n", cs.Bold("gh auth refresh -s write:public_key"))
			return cmdutil.SilentError
		}
		return err
	}

	if opts.IO.CanPrompt() && !opts.SkipConfirm {
		answer := ""
		err = prompt.SurveyAskOne(
			&survey.Input{
				Message: fmt.Sprintf("You're going to delete deploy key #%d. This action cannot be reversed. To confirm, type the title (%q):", deployKey.ID, deployKey.Title),
			},
			&answer,
		)
		if err != nil {
			return err
		}
		if err != nil || answer != deployKey.Title {
			fmt.Fprintf(opts.IO.Out, "Deploy Key #%d was not deleted.\n", deployKey.ID)
			return nil
		}
	}

	err = deleteDeployKey(httpClient, hostname, opts.ID, repo)
	if err != nil {
		if errors.Is(err, scopesError) {
			cs := opts.IO.ColorScheme()
			fmt.Fprint(opts.IO.ErrOut, "Error: insufficient OAuth scopes to delete deploy key\n")
			fmt.Fprintf(opts.IO.ErrOut, "Run the following to grant scopes: %s\n", cs.Bold("gh auth refresh -s write:public_key"))
			return cmdutil.SilentError
		}
		return err
	}

	if opts.IO.IsStdoutTTY() {
		cs := opts.IO.ColorScheme()
		fmt.Fprintf(opts.IO.Out, "%s Public key deleted from your repository\n", cs.SuccessIcon())
	}
	return nil
}
