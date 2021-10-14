package rerun

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

type RerunOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)

	RunID string

	Prompt bool
}

func NewCmdRerun(f *cmdutil.Factory, runF func(*RerunOptions) error) *cobra.Command {
	opts := &RerunOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:   "rerun [<run-id>]",
		Short: "Rerun a failed run",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if len(args) > 0 {
				opts.RunID = args[0]
			} else if !opts.IO.CanPrompt() {
				return &cmdutil.FlagError{Err: errors.New("run ID required when not running interactively")}
			} else {
				opts.Prompt = true
			}

			if runF != nil {
				return runF(opts)
			}
			return runRerun(opts)
		},
	}

	return cmd
}

func runRerun(opts *RerunOptions) error {
	c, err := opts.HttpClient()
	if err != nil {
		return fmt.Errorf("failed to create http client: %w", err)
	}
	client := api.NewClientFromHTTP(c)

	repo, err := opts.BaseRepo()
	if err != nil {
		return fmt.Errorf("failed to determine base repo: %w", err)
	}

	runID := opts.RunID

	if opts.Prompt {
		cs := opts.IO.ColorScheme()
		runs, err := shared.GetRunsWithFilter(client, repo, 10, func(run shared.Run) bool {
			if run.Status != shared.Completed {
				return false
			}
			// TODO StartupFailure indiciates a bad yaml file; such runs can never be
			// rerun. But hiding them from the prompt might confuse people?
			return run.Conclusion != shared.Success && run.Conclusion != shared.StartupFailure
		})
		if err != nil {
			return fmt.Errorf("failed to get runs: %w", err)
		}
		if len(runs) == 0 {
			return errors.New("no recent runs have failed; please specify a specific run ID")
		}
		runID, err = shared.PromptForRun(cs, runs)
		if err != nil {
			return err
		}
	}

	opts.IO.StartProgressIndicator()
	run, err := shared.GetRun(client, repo, runID)
	opts.IO.StopProgressIndicator()
	if err != nil {
		return fmt.Errorf("failed to get run: %w", err)
	}

	path := fmt.Sprintf("repos/%s/actions/runs/%d/rerun", ghrepo.FullName(repo), run.ID)

	err = client.REST(repo.RepoHost(), "POST", path, nil, nil)
	if err != nil {
		var httpError api.HTTPError
		if errors.As(err, &httpError) && httpError.StatusCode == 403 {
			return fmt.Errorf("run %d cannot be rerun; its workflow file may be broken.", run.ID)
		}
		return fmt.Errorf("failed to rerun: %w", err)
	}

	if opts.IO.CanPrompt() {
		cs := opts.IO.ColorScheme()
		fmt.Fprintf(opts.IO.Out, "%s Requested rerun of run %s\n",
			cs.SuccessIcon(),
			cs.Cyanf("%d", run.ID))
	}

	return nil
}
