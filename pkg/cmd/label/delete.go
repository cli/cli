package label

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/prompt"
	"github.com/spf13/cobra"
)

type deleteOptions struct {
	BaseRepo   func() (ghrepo.Interface, error)
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams

	Name      string
	Confirmed bool
}

func newCmdDelete(f *cmdutil.Factory, runF func(*deleteOptions) error) *cobra.Command {
	opts := deleteOptions{
		HttpClient: f.HttpClient,
		IO:         f.IOStreams,
	}

	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a label from a repository",
		Args:  cmdutil.ExactArgs(1, "cannot delete label: name argument required"),
		RunE: func(c *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo
			opts.Name = args[0]

			if !opts.IO.CanPrompt() && !opts.Confirmed {
				return cmdutil.FlagErrorf("--confirm required when not running interactively")
			}

			if runF != nil {
				return runF(&opts)
			}
			return deleteRun(&opts)
		},
	}

	cmd.Flags().BoolVar(&opts.Confirmed, "confirm", false, "Confirm deletion without prompting")

	return cmd
}

func deleteRun(opts *deleteOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	baseRepo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	if !opts.Confirmed {
		var valid string
		err := prompt.SurveyAskOne(
			&survey.Input{Message: fmt.Sprintf("Type %s to confirm deletion:", opts.Name)},
			&valid,
			survey.WithValidator(
				func(val interface{}) error {
					if str := val.(string); !strings.EqualFold(str, opts.Name) {
						return fmt.Errorf("You entered %s", str)
					}
					return nil
				}))
		if err != nil {
			return fmt.Errorf("could not prompt: %w", err)
		}
	}

	opts.IO.StartProgressIndicator()
	err = deleteLabel(httpClient, baseRepo, opts.Name)
	opts.IO.StopProgressIndicator()
	if err != nil {
		return err
	}

	if opts.IO.IsStdoutTTY() {
		cs := opts.IO.ColorScheme()
		successMsg := fmt.Sprintf("%s Label %q deleted from %s\n", cs.SuccessIcon(), opts.Name, ghrepo.FullName(baseRepo))
		fmt.Fprint(opts.IO.Out, successMsg)
	}

	return nil
}

func deleteLabel(client *http.Client, repo ghrepo.Interface, name string) error {
	apiClient := api.NewClientFromHTTP(client)
	path := fmt.Sprintf("repos/%s/%s/labels/%s", repo.RepoOwner(), repo.RepoName(), name)

	return apiClient.REST(repo.RepoHost(), "DELETE", path, nil, nil)
}
