package delete

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/prompt"

	"github.com/AlecAivazis/survey/v2"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type DeleteOptions struct {
	HttpClient func() (*http.Client, error)
	BaseRepo   func() (ghrepo.Interface, error)
	IO         *iostreams.IOStreams
	RepoArg    string
	Confirmed  bool
}

func NewCmdDelete(f *cmdutil.Factory, runF func(*DeleteOptions) error) *cobra.Command {
	opts := &DeleteOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		BaseRepo:   f.BaseRepo,
	}

	cmd := &cobra.Command{
		Use:   "delete [<repository>]",
		Short: "Delete a repository",
		Long: `Delete a GitHub repository.

With no argument, deletes the current repository. Otherwise, deletes the specified repository.

Deletion requires authorization with the "delete_repo" scope. 
To authorize, run "gh auth refresh -s delete_repo"`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.RepoArg = args[0]
			}
			if !opts.IO.CanPrompt() && !opts.Confirmed {
				return cmdutil.FlagErrorf("--confirm required when not running interactively")
			}

			if runF != nil {
				return runF(opts)
			}
			return deleteRun(opts)
		},
	}

	cmd.Flags().BoolVar(&opts.Confirmed, "confirm", false, "confirm deletion without prompting")
	return cmd
}

func deleteRun(opts *DeleteOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	apiClient := api.NewClientFromHTTP(httpClient)

	var toDelete ghrepo.Interface

	if opts.RepoArg == "" {
		toDelete, err = opts.BaseRepo()
		if err != nil {
			return err
		}
	} else {
		repoSelector := opts.RepoArg
		if !strings.Contains(repoSelector, "/") {
			currentUser, err := api.CurrentLoginName(apiClient, ghinstance.Default())
			if err != nil {
				return err
			}
			repoSelector = currentUser + "/" + repoSelector
		}
		toDelete, err = ghrepo.FromFullName(repoSelector)
		if err != nil {
			return fmt.Errorf("argument error: %w", err)
		}
	}
	fullName := ghrepo.FullName(toDelete)

	if !opts.Confirmed {
		var valid string
		err := prompt.SurveyAskOne(
			&survey.Input{Message: fmt.Sprintf("Type %s to confirm deletion:", fullName)},
			&valid,
			survey.WithValidator(
				func(val interface{}) error {
					if str := val.(string); !strings.EqualFold(str, fullName) {
						return fmt.Errorf("You entered %s", str)
					}
					return nil
				}))
		if err != nil {
			return fmt.Errorf("could not prompt: %w", err)
		}
	}

	err = deleteRepo(httpClient, toDelete)
	if err != nil {
		return err
	}

	if opts.IO.IsStdoutTTY() {
		cs := opts.IO.ColorScheme()
		fmt.Fprintf(opts.IO.Out,
			"%s Deleted repository %s\n",
			cs.SuccessIcon(),
			fullName)
	}

	return nil
}
