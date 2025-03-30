package delete

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/cmd/run/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

const (
	defaultRunGetLimit = 10
)

type DeleteOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Prompter   prompter.Prompter
	Prompt     bool
	RunID      string
}

func NewCmdDelete(f *cmdutil.Factory, runF func(*DeleteOptions) error) *cobra.Command {
	opts := &DeleteOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Prompter:   f.Prompter,
	}

	cmd := &cobra.Command{
		Use:   "delete [<run-id>]",
		Short: "Delete a workflow run",
		Example: heredoc.Doc(`
			# Interactively select a run to delete
			$ gh run delete

			# Delete a specific run
			$ gh run delete 12345
		`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if len(args) > 0 {
				opts.RunID = args[0]
			} else if !opts.IO.CanPrompt() {
				return cmdutil.FlagErrorf("run ID required when not running interactively")
			} else {
				opts.Prompt = true
			}

			if runF != nil {
				return runF(opts)
			}

			return runDelete(opts)
		},
	}

	return cmd
}

func runDelete(opts *DeleteOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return fmt.Errorf("failed to create http client: %w", err)
	}
	client := api.NewClientFromHTTP(httpClient)

	cs := opts.IO.ColorScheme()

	repo, err := opts.BaseRepo()
	if err != nil {
		return fmt.Errorf("failed to determine base repo: %w", err)
	}

	runID := opts.RunID
	var run *shared.Run

	if opts.Prompt {
		payload, err := shared.GetRuns(client, repo, nil, defaultRunGetLimit)
		if err != nil {
			return fmt.Errorf("failed to get runs: %w", err)
		}

		runs := payload.WorkflowRuns
		if len(runs) == 0 {
			return fmt.Errorf("found no runs to delete")
		}

		runID, err = shared.SelectRun(opts.Prompter, cs, runs)
		if err != nil {
			return err
		}

		for _, r := range runs {
			if fmt.Sprintf("%d", r.ID) == runID {
				run = &r
				break
			}
		}
	} else {
		run, err = shared.GetRun(client, repo, runID, 0)
		if err != nil {
			var httpErr api.HTTPError
			if errors.As(err, &httpErr) {
				if httpErr.StatusCode == http.StatusNotFound {
					err = fmt.Errorf("could not find any workflow run with ID %s", opts.RunID)
				}
			}
			return err
		}
	}

	err = deleteWorkflowRun(client, repo, fmt.Sprintf("%d", run.ID))
	if err != nil {
		var httpErr api.HTTPError
		if errors.As(err, &httpErr) {
			if httpErr.StatusCode == http.StatusConflict {
				err = fmt.Errorf("cannot delete a workflow run that is completed")
			}
		}

		return err
	}

	fmt.Fprintf(opts.IO.Out, "%s Request to delete workflow run submitted.\n", cs.SuccessIcon())
	return nil
}

func deleteWorkflowRun(client *api.Client, repo ghrepo.Interface, runID string) error {
	path := fmt.Sprintf("repos/%s/actions/runs/%s", ghrepo.FullName(repo), runID)
	err := client.REST(repo.RepoHost(), "DELETE", path, nil, nil)
	if err != nil {
		return err
	}
	return nil
}
