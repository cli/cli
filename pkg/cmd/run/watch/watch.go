package watch

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmd/run/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

const defaultInterval int = 3

type WatchOptions struct {
	IO         *iostreams.IOStreams
	HttpClient func() (*http.Client, error)
	BaseRepo   func() (ghrepo.Interface, error)

	RunID      string
	Interval   int
	ExitStatus bool

	Prompt bool

	Now func() time.Time
}

func NewCmdWatch(f *cmdutil.Factory, runF func(*WatchOptions) error) *cobra.Command {
	opts := &WatchOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Now:        time.Now,
	}

	cmd := &cobra.Command{
		Use:   "watch <run-id>",
		Short: "Watch a run until it completes, showing its progress",
		Example: heredoc.Doc(`
			# Watch a run until it's done
			gh run watch

			# Run some other command when the run is finished
			gh run watch && notify-send "run is done!"
		`),
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

			return watchRun(opts)
		},
	}
	cmd.Flags().BoolVar(&opts.ExitStatus, "exit-status", false, "Exit with non-zero status if run fails")
	cmd.Flags().IntVarP(&opts.Interval, "interval", "i", defaultInterval, "Refresh interval in seconds")

	return cmd
}

func watchRun(opts *WatchOptions) error {
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
	var run *shared.Run

	if opts.Prompt {
		runs, err := shared.GetRunsWithFilter(client, repo, nil, 10, func(run shared.Run) bool {
			return run.Status != shared.Completed
		})
		if err != nil {
			return fmt.Errorf("failed to get runs: %w", err)
		}
		if len(runs) == 0 {
			return fmt.Errorf("found no in progress runs to watch")
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
			return fmt.Errorf("failed to get run: %w", err)
		}
	}

	if run.Status == shared.Completed {
		fmt.Fprintf(opts.IO.Out, "Run %s (%s) has already completed with '%s'\n", cs.Bold(run.WorkflowName()), cs.Cyanf("%d", run.ID), run.Conclusion)
		if opts.ExitStatus && run.Conclusion != shared.Success {
			return cmdutil.SilentError
		}
		return nil
	}

	prNumber := ""
	number, err := shared.PullRequestForRun(client, repo, *run)
	if err == nil {
		prNumber = fmt.Sprintf(" #%d", number)
	}

	annotationCache := map[int64][]shared.Annotation{}

	duration, err := time.ParseDuration(fmt.Sprintf("%ds", opts.Interval))
	if err != nil {
		return fmt.Errorf("could not parse interval: %w", err)
	}

	out := &bytes.Buffer{}
	opts.IO.StartAlternateScreenBuffer()
	for run.Status != shared.Completed {
		// Write to a temporary buffer to reduce total number of fetches
		run, err = renderRun(out, *opts, client, repo, run, prNumber, annotationCache)
		if err != nil {
			break
		}

		if run.Status == shared.Completed {
			break
		}

		// If not completed, refresh the screen buffer and write the temporary buffer to stdout
		opts.IO.RefreshScreen()

		fmt.Fprintln(opts.IO.Out, cs.Boldf("Refreshing run status every %d seconds. Press Ctrl+C to quit.", opts.Interval))
		fmt.Fprintln(opts.IO.Out)

		_, err = io.Copy(opts.IO.Out, out)
		out.Reset()

		if err != nil {
			break
		}

		time.Sleep(duration)
	}
	opts.IO.StopAlternateScreenBuffer()

	if err != nil {
		return nil
	}

	// Write the last temporary buffer one last time
	_, err = io.Copy(opts.IO.Out, out)
	if err != nil {
		return err
	}

	symbol, symbolColor := shared.Symbol(cs, run.Status, run.Conclusion)
	id := cs.Cyanf("%d", run.ID)

	if opts.IO.IsStdoutTTY() {
		fmt.Fprintln(opts.IO.Out)
		fmt.Fprintf(opts.IO.Out, "%s Run %s (%s) completed with '%s'\n", symbolColor(symbol), cs.Bold(run.WorkflowName()), id, run.Conclusion)
	}

	if opts.ExitStatus && run.Conclusion != shared.Success {
		return cmdutil.SilentError
	}

	return nil
}

func renderRun(out io.Writer, opts WatchOptions, client *api.Client, repo ghrepo.Interface, run *shared.Run, prNumber string, annotationCache map[int64][]shared.Annotation) (*shared.Run, error) {
	cs := opts.IO.ColorScheme()

	var err error

	run, err = shared.GetRun(client, repo, fmt.Sprintf("%d", run.ID), 0)
	if err != nil {
		return nil, fmt.Errorf("failed to get run: %w", err)
	}

	jobs, err := shared.GetJobs(client, repo, run)
	if err != nil {
		return nil, fmt.Errorf("failed to get jobs: %w", err)
	}

	var annotations []shared.Annotation

	var annotationErr error
	var as []shared.Annotation
	for _, job := range jobs {
		if as, ok := annotationCache[job.ID]; ok {
			annotations = as
			continue
		}

		as, annotationErr = shared.GetAnnotations(client, repo, job)
		if annotationErr != nil {
			break
		}
		annotations = append(annotations, as...)

		if job.Status != shared.InProgress {
			annotationCache[job.ID] = annotations
		}
	}

	if annotationErr != nil {
		return nil, fmt.Errorf("failed to get annotations: %w", annotationErr)
	}

	fmt.Fprintln(out, shared.RenderRunHeader(cs, *run, text.FuzzyAgo(opts.Now(), run.StartedTime()), prNumber, 0))
	fmt.Fprintln(out)

	if len(jobs) == 0 {
		return run, nil
	}

	fmt.Fprintln(out, cs.Bold("JOBS"))

	fmt.Fprintln(out, shared.RenderJobs(cs, jobs, true))

	if len(annotations) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, cs.Bold("ANNOTATIONS"))
		fmt.Fprintln(out, shared.RenderAnnotations(cs, annotations))
	}

	return run, nil
}
