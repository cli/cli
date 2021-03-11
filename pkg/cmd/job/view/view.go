package view

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmd/run/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/prompt"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

type ViewOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)

	JobID      string
	Log        bool
	ExitStatus bool

	Prompt bool

	Now func() time.Time
}

func NewCmdView(f *cmdutil.Factory, runF func(*ViewOptions) error) *cobra.Command {
	opts := &ViewOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Now:        time.Now,
	}
	cmd := &cobra.Command{
		Use:    "view [<job-id>]",
		Short:  "View the summary or full logs of a workflow run's job",
		Args:   cobra.MaximumNArgs(1),
		Hidden: true,
		Example: heredoc.Doc(`
			# Interactively select a run then job
			$ gh job view

			# Just view the logs for a job
			$ gh job view 0451 --log

			# Exit non-zero if a job failed
			$ gh job view 0451 -e && echo "job pending or passed"
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if len(args) > 0 {
				opts.JobID = args[0]
			} else if !opts.IO.CanPrompt() {
				return &cmdutil.FlagError{Err: errors.New("job ID required when not running interactively")}
			} else {
				opts.Prompt = true
			}

			if runF != nil {
				return runF(opts)
			}
			return runView(opts)
		},
	}
	cmd.Flags().BoolVarP(&opts.Log, "log", "l", false, "Print full logs for job")
	// TODO should we try and expose pending via another exit code?
	cmd.Flags().BoolVar(&opts.ExitStatus, "exit-status", false, "Exit with non-zero status if job failed")

	return cmd
}

func runView(opts *ViewOptions) error {
	c, err := opts.HttpClient()
	if err != nil {
		return fmt.Errorf("failed to create http client: %w", err)
	}
	client := api.NewClientFromHTTP(c)

	repo, err := opts.BaseRepo()
	if err != nil {
		return fmt.Errorf("failed to determine base repo: %w", err)
	}

	out := opts.IO.Out
	cs := opts.IO.ColorScheme()

	jobID := opts.JobID
	if opts.Prompt {
		runID, err := shared.PromptForRun(cs, client, repo)
		if err != nil {
			return err
		}
		// TODO I'd love to overwrite the result of the prompt since it adds visual noise but I'm not sure
		// the cleanest way to do that.
		fmt.Fprintln(out)

		opts.IO.StartProgressIndicator()
		defer opts.IO.StopProgressIndicator()

		run, err := shared.GetRun(client, repo, runID)
		if err != nil {
			return fmt.Errorf("failed to get run: %w", err)
		}

		opts.IO.StopProgressIndicator()
		jobID, err = promptForJob(*opts, client, repo, *run)
		if err != nil {
			return err
		}

		fmt.Fprintln(out)
	}

	opts.IO.StartProgressIndicator()
	job, err := getJob(client, repo, jobID)
	if err != nil {
		return fmt.Errorf("failed to get job: %w", err)
	}

	if opts.Log {
		r, err := client.JobLog(repo, jobID)
		if err != nil {
			return err
		}

		opts.IO.StopProgressIndicator()

		opts.IO.DetectTerminalTheme()
		err = opts.IO.StartPager()
		if err != nil {
			return err
		}
		defer opts.IO.StopPager()

		if _, err := io.Copy(opts.IO.Out, r); err != nil {
			return fmt.Errorf("failed to read log: %w", err)
		}

		if opts.ExitStatus && shared.IsFailureState(job.Conclusion) {
			return cmdutil.SilentError
		}

		return nil
	}

	annotations, err := shared.GetAnnotations(client, repo, *job)
	opts.IO.StopProgressIndicator()
	if err != nil {
		return fmt.Errorf("failed to get annotations: %w", err)
	}

	elapsed := job.CompletedAt.Sub(job.StartedAt)
	elapsedStr := fmt.Sprintf(" in %s", elapsed)
	if elapsed < 0 {
		elapsedStr = ""
	}

	symbol, symColor := shared.Symbol(cs, job.Status, job.Conclusion)

	fmt.Fprintf(out, "%s (ID %s)\n", cs.Bold(job.Name), cs.Cyanf("%d", job.ID))
	fmt.Fprintf(out, "%s %s ago%s\n",
		symColor(symbol),
		utils.FuzzyAgoAbbr(opts.Now(), job.StartedAt),
		elapsedStr)

	fmt.Fprintln(out)

	for _, step := range job.Steps {
		stepSym, stepSymColor := shared.Symbol(cs, step.Status, step.Conclusion)
		fmt.Fprintf(out, "%s %s\n",
			stepSymColor(stepSym),
			step.Name)
	}

	if len(annotations) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, cs.Bold("ANNOTATIONS"))

		for _, a := range annotations {
			fmt.Fprintf(out, "%s %s\n", shared.AnnotationSymbol(cs, a), a.Message)
			fmt.Fprintln(out, cs.Grayf("%s#%d\n", a.Path, a.StartLine))
		}
	}

	fmt.Fprintln(out)
	fmt.Fprintf(out, "To see the full logs for this job, try: gh job view %s --log\n", jobID)
	fmt.Fprintf(out, cs.Gray("View this job on GitHub: %s\n"), job.URL)

	if opts.ExitStatus && shared.IsFailureState(job.Conclusion) {
		return cmdutil.SilentError
	}

	return nil
}

func getJob(client *api.Client, repo ghrepo.Interface, jobID string) (*shared.Job, error) {
	path := fmt.Sprintf("repos/%s/actions/jobs/%s", ghrepo.FullName(repo), jobID)

	var result shared.Job
	err := client.REST(repo.RepoHost(), "GET", path, nil, &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

func promptForJob(opts ViewOptions, client *api.Client, repo ghrepo.Interface, run shared.Run) (string, error) {
	cs := opts.IO.ColorScheme()
	jobs, err := shared.GetJobs(client, repo, run)
	if err != nil {
		return "", err
	}

	if len(jobs) == 1 {
		return fmt.Sprintf("%d", jobs[0].ID), nil
	}

	var selected int

	candidates := []string{}

	for _, job := range jobs {
		symbol, symColor := shared.Symbol(cs, job.Status, job.Conclusion)
		candidates = append(candidates, fmt.Sprintf("%s %s", symColor(symbol), job.Name))
	}

	// TODO consider custom filter so it's fuzzier. right now matches start anywhere in string but
	// become contiguous
	err = prompt.SurveyAskOne(&survey.Select{
		Message:  "Select a job to view",
		Options:  candidates,
		PageSize: 10,
	}, &selected)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%d", jobs[selected].ID), nil
}
