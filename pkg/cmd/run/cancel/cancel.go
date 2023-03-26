package cancel

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/run/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type CancelOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)

	Prompt bool

	RunID string
}

func NewCmdCancel(f *cmdutil.Factory, runF func(*CancelOptions) error) *cobra.Command {
	opts := &CancelOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:   "cancel [<run-id>]",
		Short: "Cancel a workflow run",
		Args:  cobra.MaximumNArgs(1),
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

			return runCancel(opts)
		},
	}

	return cmd
}

func runCancel(opts *CancelOptions) error {
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
		runs, err := shared.GetRunsWithFilter(client, repo, nil, 10, func(run shared.Run) bool {
			return run.Status != shared.Completed
		})
		if err != nil {
			return fmt.Errorf("failed to get runs: %w", err)
		}
		if len(runs) == 0 {
			return fmt.Errorf("found no in progress runs to cancel")
		}
		runID, err = shared.PromptForRun(cs, runs)
		if err != nil {
			return err
		}
		// TODO silly stopgap until dust settles and PromptForRun can just return a run
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
					err = fmt.Errorf("Could not find any workflow run with ID %s", opts.RunID)
				}
			}
			return err
		}
	}

	err = cancelWorkflowRun(client, repo, fmt.Sprintf("%d", run.ID))
	if err != nil {
		var httpErr api.HTTPError
		if errors.As(err, &httpErr) {
			if httpErr.StatusCode == http.StatusConflict {
				err = fmt.Errorf("Cannot cancel a workflow run that is completed")
			}
		}

		return err
	}

	fmt.Fprintf(opts.IO.Out, "%s Request to cancel workflow submitted.\n", cs.SuccessIcon())

	return nil
}

func cancelWorkflowRun(client *api.Client, repo ghrepo.Interface, runID string) error {
	path := fmt.Sprintf("repos/%s/actions/runs/%s/cancel", ghrepo.FullName(repo), runID)

	err := client.REST(repo.RepoHost(), "POST", path, nil, nil)
	if err != nil {
		return err
	}

	return nil
}
