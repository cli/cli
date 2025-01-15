package rerun

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/MakeNowJust/heredoc"
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
	Prompter   shared.Prompter

	RunID      string
	OnlyFailed bool
	JobID      string
	Debug      bool

	Prompt bool
}

func NewCmdRerun(f *cmdutil.Factory, runF func(*RerunOptions) error) *cobra.Command {
	opts := &RerunOptions{
		IO:         f.IOStreams,
		Prompter:   f.Prompter,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:   "rerun [<run-id>]",
		Short: "Rerun a run",
		Long: heredoc.Docf(`
			Rerun an entire run, only failed jobs, or a specific job from a run.

			Note that due to historical reasons, the %[1]s--job%[1]s flag may not take what you expect.
			Specifically, when navigating to a job in the browser, the URL looks like this:
			%[1]shttps://github.com/<owner>/<repo>/actions/runs/<run-id>/jobs/<number>%[1]s.

			However, this %[1]s<number>%[1]s should not be used with the %[1]s--job%[1]s flag and will result in the
			API returning %[1]s404 NOT FOUND%[1]s. Instead, you can get the correct job IDs using the following command:
			
				gh run view <run-id> --json jobs --jq '.jobs[] | {name, databaseId}'

			You will need to use databaseId field for triggering job re-runs.
		`, "`"),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if len(args) == 0 && opts.JobID == "" {
				if !opts.IO.CanPrompt() {
					return cmdutil.FlagErrorf("`<run-id>` or `--job` required when not running interactively")
				} else {
					opts.Prompt = true
				}
			} else if len(args) > 0 {
				opts.RunID = args[0]
			}

			if opts.RunID != "" && opts.JobID != "" {
				opts.RunID = ""
				if opts.IO.CanPrompt() {
					cs := opts.IO.ColorScheme()
					fmt.Fprintf(opts.IO.ErrOut, "%s both run and job IDs specified; ignoring run ID\n", cs.WarningIcon())
				}
			}

			if runF != nil {
				return runF(opts)
			}
			return runRerun(opts)
		},
	}

	cmd.Flags().BoolVar(&opts.OnlyFailed, "failed", false, "Rerun only failed jobs, including dependencies")
	cmd.Flags().StringVarP(&opts.JobID, "job", "j", "", "Rerun a specific job ID from a run, including dependencies")
	cmd.Flags().BoolVarP(&opts.Debug, "debug", "d", false, "Rerun with debug logging")

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

	cs := opts.IO.ColorScheme()

	runID := opts.RunID
	jobID := opts.JobID
	var selectedJob *shared.Job

	if jobID != "" {
		opts.IO.StartProgressIndicator()
		selectedJob, err = shared.GetJob(client, repo, jobID)
		opts.IO.StopProgressIndicator()
		if err != nil {
			return fmt.Errorf("failed to get job: %w", err)
		}
		runID = fmt.Sprintf("%d", selectedJob.RunID)
	}

	if opts.Prompt {
		runs, err := shared.GetRunsWithFilter(client, repo, nil, 10, func(run shared.Run) bool {
			if run.Status != shared.Completed {
				return false
			}
			// TODO StartupFailure indicates a bad yaml file; such runs can never be
			// rerun. But hiding them from the prompt might confuse people?
			return run.Conclusion != shared.Success && run.Conclusion != shared.StartupFailure
		})
		if err != nil {
			return fmt.Errorf("failed to get runs: %w", err)
		}
		if len(runs) == 0 {
			return errors.New("no recent runs have failed; please specify a specific `<run-id>`")
		}
		if runID, err = shared.SelectRun(opts.Prompter, cs, runs); err != nil {
			return err
		}
	}

	debugMsg := ""
	if opts.Debug {
		debugMsg = " with debug logging enabled"
	}

	if opts.JobID != "" {
		err = rerunJob(client, repo, selectedJob, opts.Debug)
		if err != nil {
			return err
		}
		if opts.IO.IsStdoutTTY() {
			fmt.Fprintf(opts.IO.Out, "%s Requested rerun of job %s on run %s%s\n",
				cs.SuccessIcon(),
				cs.Cyanf("%d", selectedJob.ID),
				cs.Cyanf("%d", selectedJob.RunID),
				debugMsg)
		}
	} else {
		opts.IO.StartProgressIndicator()
		run, err := shared.GetRun(client, repo, runID, 0)
		opts.IO.StopProgressIndicator()
		if err != nil {
			return fmt.Errorf("failed to get run: %w", err)
		}

		err = rerunRun(client, repo, run, opts.OnlyFailed, opts.Debug)
		if err != nil {
			return err
		}
		if opts.IO.IsStdoutTTY() {
			onlyFailedMsg := ""
			if opts.OnlyFailed {
				onlyFailedMsg = "(failed jobs) "
			}
			fmt.Fprintf(opts.IO.Out, "%s Requested rerun %sof run %s%s\n",
				cs.SuccessIcon(),
				onlyFailedMsg,
				cs.Cyanf("%d", run.ID),
				debugMsg)
		}
	}

	return nil
}

func rerunRun(client *api.Client, repo ghrepo.Interface, run *shared.Run, onlyFailed, debug bool) error {
	runVerb := "rerun"
	if onlyFailed {
		runVerb = "rerun-failed-jobs"
	}

	body, err := requestBody(debug)
	if err != nil {
		return fmt.Errorf("failed to create rerun body: %w", err)
	}

	path := fmt.Sprintf("repos/%s/actions/runs/%d/%s", ghrepo.FullName(repo), run.ID, runVerb)

	err = client.REST(repo.RepoHost(), "POST", path, body, nil)
	if err != nil {
		var httpError api.HTTPError
		if errors.As(err, &httpError) && httpError.StatusCode == 403 {
			return fmt.Errorf("run %d cannot be rerun; its workflow file may be broken", run.ID)
		}
		return fmt.Errorf("failed to rerun: %w", err)
	}
	return nil
}

func rerunJob(client *api.Client, repo ghrepo.Interface, job *shared.Job, debug bool) error {
	body, err := requestBody(debug)
	if err != nil {
		return fmt.Errorf("failed to create rerun body: %w", err)
	}

	path := fmt.Sprintf("repos/%s/actions/jobs/%d/rerun", ghrepo.FullName(repo), job.ID)

	err = client.REST(repo.RepoHost(), "POST", path, body, nil)
	if err != nil {
		var httpError api.HTTPError
		if errors.As(err, &httpError) && httpError.StatusCode == 403 {
			return fmt.Errorf("job %d cannot be rerun", job.ID)
		}
		return fmt.Errorf("failed to rerun: %w", err)
	}
	return nil
}

type RerunPayload struct {
	Debug bool `json:"enable_debug_logging"`
}

func requestBody(debug bool) (io.Reader, error) {
	if !debug {
		return nil, nil
	}
	params := &RerunPayload{
		debug,
	}

	body := &bytes.Buffer{}
	enc := json.NewEncoder(body)
	if err := enc.Encode(params); err != nil {
		return nil, err
	}
	return body, nil
}
