package view

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmd/run/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

type ViewOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)

	RunID   string
	Verbose bool

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
		Use:   "view [<run-id>]",
		Short: "View a summary of a workflow run",
		// TODO examples?
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			terminal := opts.IO.IsStdoutTTY() && opts.IO.IsStdinTTY()
			opts.ShowProgress = terminal

			if len(args) > 0 {
				opts.RunID = args[0]
			} else if !terminal {
				return &cmdutil.FlagError{Err: errors.New("expected a run ID")}
			} else {
				opts.Prompt = true
			}

			if runF != nil {
				return runF(opts)
			}
			return runView(opts)
		},
	}
	cmd.Flags().BoolVarP(&opts.Verbose, "verbose", "v", false, "Show job steps")

	return cmd
}

func runView(opts *ViewOptions) error {
	c, err := opts.HttpClient()
	if err != nil {
		// TODO error handle
		return err
	}
	client := api.NewClientFromHTTP(c)

	repo, err := opts.BaseRepo()
	if err != nil {
		// TODO error handle
		return err
	}

	runID := opts.RunID

	if opts.Prompt {
		cs := opts.IO.ColorScheme()
		runID, err = shared.PromptForRun(cs, client, repo)
		if err != nil {
			// TODO error handle
			return err
		}
	}

	if opts.ShowProgress {
		opts.IO.StartProgressIndicator()
	}
	run, err := shared.GetRun(client, repo, runID)
	if err != nil {
		// TODO error handle
		return err
	}

	jobs, err := shared.GetJobs(client, repo, *run)
	if err != nil {
		// TODO error handle
		return err
	}

	var annotations []shared.Annotation

	var annotationErr error
	var as []shared.Annotation
	for _, job := range jobs {
		as, annotationErr = shared.GetAnnotations(client, repo, job)
		if annotationErr != nil {
			break
		}
		annotations = append(annotations, as...)
	}

	if annotationErr != nil {
		// TODO handle error
		return annotationErr
	}

	if opts.ShowProgress {
		opts.IO.StopProgressIndicator()
	}
	err = renderRun(*opts, *run, jobs, annotations)
	if err != nil {
		// TODO handle error
		return err
	}

	return nil
}

func titleForRun(cs *iostreams.ColorScheme, run shared.Run) string {
	// TODO how to obtain? i can get a SHA but it's not immediately clear how to get from sha -> pr
	// without a ton of hops
	prID := ""

	return fmt.Sprintf("%s %s%s",
		cs.Bold(run.HeadBranch),
		run.Name,
		prID)
}

// TODO consider context struct for all this:

func renderRun(opts ViewOptions, run shared.Run, jobs []shared.Job, annotations []shared.Annotation) error {
	out := opts.IO.Out
	cs := opts.IO.ColorScheme()

	title := titleForRun(cs, run)
	symbol := shared.Symbol(cs, run.Status, run.Conclusion)
	id := cs.Cyanf("%d", run.ID)

	fmt.Fprintf(out, "%s %s · %s\n", symbol, title, id)

	ago := opts.Now().Sub(run.CreatedAt)

	fmt.Fprintf(out, "Triggered via %s %s\n", run.Event, utils.FuzzyAgo(ago))
	fmt.Fprintln(out)
	fmt.Fprintln(out, cs.Bold("JOBS"))

	for _, job := range jobs {
		symbol := shared.Symbol(cs, job.Status, job.Conclusion)
		id := cs.Cyanf("%d", job.ID)
		fmt.Fprintf(out, "%s %s (ID %s)\n", symbol, job.Name, id)
		if opts.Verbose || shared.IsFailureState(job.Conclusion) {
			for _, step := range job.Steps {
				fmt.Fprintf(out, "  %s %s\n",
					shared.Symbol(cs, step.Status, step.Conclusion),
					step.Name)
			}
		}
	}

	if len(annotations) == 0 {
		return nil
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, cs.Bold("ANNOTATIONS"))

	for _, a := range annotations {
		fmt.Fprintf(out, "%s %s\n", a.Symbol(cs), a.Message)
		fmt.Fprintln(out, cs.Grayf("%s: %s#%d\n",
			a.JobName, a.Path, a.StartLine))
	}

	fmt.Fprintln(out, "For more information about a job, try: gh job view <job-id>")

	return nil
}
