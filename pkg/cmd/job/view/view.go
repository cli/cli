package view

import (
	"errors"
	"fmt"
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

	Prompt       bool
	ShowProgress bool

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

			terminal := opts.IO.IsStdoutTTY() && opts.IO.IsStdinTTY()
			opts.ShowProgress = terminal

			if len(args) > 0 {
				opts.JobID = args[0]
			} else if !terminal {
				return &cmdutil.FlagError{Err: errors.New("expected a job ID")}
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
	cmd.Flags().BoolVarP(&opts.ExitStatus, "exit-status", "e", false, "Exit with non-zero status if job failed")

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

		if opts.ShowProgress {
			opts.IO.StartProgressIndicator()
		}

		run, err := shared.GetRun(client, repo, runID)
		if err != nil {
			return fmt.Errorf("failed to get run: %w", err)
		}

		if opts.ShowProgress {
			opts.IO.StopProgressIndicator()
		}

		jobID, err = promptForJob(*opts, client, repo, *run)
		if err != nil {
			return err
		}

		fmt.Fprintln(out)
	}

	if opts.ShowProgress {
		opts.IO.StartProgressIndicator()
	}

	job, err := getJob(client, repo, jobID)
	if err != nil {
		return fmt.Errorf("failed to get job: %w", err)
	}

	if opts.Log {
		r, err := client.JobLog(repo, jobID)
		if err != nil {
			return err
		}

		for {
			buff := make([]byte, 64)
			n, err := r.Read(buff)
			if n <= 0 {
				break
			}
			fmt.Fprint(out, string(buff[0:n]))
			if err != nil {
				break
			}
		}

		if opts.ExitStatus && shared.IsFailureState(job.Conclusion) {
			return cmdutil.SilentError
		}

		return nil
	}

	annotations, err := shared.GetAnnotations(client, repo, *job)
	if err != nil {
		return fmt.Errorf("failed to get annotations: %w", err)
	}

	if opts.ShowProgress {
		opts.IO.StopProgressIndicator()
	}

	ago := opts.Now().Sub(job.StartedAt)
	elapsed := job.CompletedAt.Sub(job.StartedAt)

	elapsedStr := fmt.Sprintf(" in %s", elapsed)

	if elapsed < 0 {
		elapsedStr = ""
	}

	fmt.Fprintf(out, "%s (ID %s)\n", cs.Bold(job.Name), cs.Cyanf("%d", job.ID))
	fmt.Fprintf(out, "%s %s%s\n",
		shared.Symbol(cs, job.Status, job.Conclusion),
		utils.FuzzyAgo(ago),
		elapsedStr)

	fmt.Fprintln(out)

	for _, step := range job.Steps {
		fmt.Fprintf(out, "%s %s\n",
			shared.Symbol(cs, step.Status, step.Conclusion),
			step.Name)
	}

	if len(annotations) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, cs.Bold("ANNOTATIONS"))

		for _, a := range annotations {
			fmt.Fprintf(out, "%s %s\n", a.Symbol(cs), a.Message)
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
		symbol := shared.Symbol(cs, job.Status, job.Conclusion)
		candidates = append(candidates, fmt.Sprintf("%s %s", symbol, job.Name))
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
