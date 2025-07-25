package view

import (
	"archive/zip"
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmd/run/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type RunLogCache struct {
	cacheDir string
}

func (c RunLogCache) Exists(key string) (bool, error) {
	_, err := os.Stat(c.filepath(key))
	if err == nil {
		return true, nil
	}

	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}

	return false, fmt.Errorf("checking cache entry: %v", err)
}

func (c RunLogCache) Create(key string, content io.Reader) error {
	if err := os.MkdirAll(c.cacheDir, 0755); err != nil {
		return fmt.Errorf("creating cache directory: %v", err)
	}

	out, err := os.Create(c.filepath(key))
	if err != nil {
		return fmt.Errorf("creating cache entry: %v", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, content); err != nil {
		return fmt.Errorf("writing cache entry: %v", err)

	}

	return nil
}

func (c RunLogCache) Open(key string) (*zip.ReadCloser, error) {
	r, err := zip.OpenReader(c.filepath(key))
	if err != nil {
		return nil, fmt.Errorf("opening cache entry: %v", err)
	}

	return r, nil
}

func (c RunLogCache) filepath(key string) string {
	return filepath.Join(c.cacheDir, fmt.Sprintf("run-log-%s.zip", key))
}

type ViewOptions struct {
	HttpClient  func() (*http.Client, error)
	IO          *iostreams.IOStreams
	BaseRepo    func() (ghrepo.Interface, error)
	Browser     browser.Browser
	Prompter    shared.Prompter
	RunLogCache RunLogCache

	RunID      string
	JobID      string
	Verbose    bool
	ExitStatus bool
	Log        bool
	LogFailed  bool
	Web        bool
	Attempt    uint64

	Prompt   bool
	Exporter cmdutil.Exporter

	Now func() time.Time
}

func NewCmdView(f *cmdutil.Factory, runF func(*ViewOptions) error) *cobra.Command {
	opts := &ViewOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Prompter:   f.Prompter,
		Now:        time.Now,
		Browser:    f.Browser,
	}

	cmd := &cobra.Command{
		Use:   "view [<run-id>]",
		Short: "View a summary of a workflow run",
		Long: heredoc.Docf(`
			View a summary of a workflow run.

			Due to platform limitations, %[1]sgh%[1]s may not always be able to associate jobs with their
			corresponding logs when using the primary method of fetching logs in zip format.

			In such cases, %[1]sgh%[1]s will attempt to fetch logs for each job individually via the API.
			This fallback is slower and more resource-intensive. If more than 25 job logs are missing,
			the operation will fail with an error.

			Additionally, due to similar platform constraints, some log lines may not be
			associated with a specific step within a job. In these cases, the step name will
			appear as %[1]sUNKNOWN STEP%[1]s in the log output.
		`, "`", maxAPILogFetchers),
		Args: cobra.MaximumNArgs(1),
		Example: heredoc.Doc(`
			# Interactively select a run to view, optionally selecting a single job
			$ gh run view

			# View a specific run
			$ gh run view 12345

			# View a specific run with specific attempt number
			$ gh run view 12345 --attempt 3

			# View a specific job within a run
			$ gh run view --job 456789

			# View the full log for a specific job
			$ gh run view --log --job 456789

			# Exit non-zero if a run failed
			$ gh run view 0451 --exit-status && echo "run pending or passed"
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			config, err := f.Config()
			if err != nil {
				return err
			}

			opts.RunLogCache = RunLogCache{
				cacheDir: config.CacheDir(),
			}

			if len(args) == 0 && opts.JobID == "" {
				if !opts.IO.CanPrompt() {
					return cmdutil.FlagErrorf("run or job ID required when not running interactively")
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

			if opts.Web && opts.Log {
				return cmdutil.FlagErrorf("specify only one of --web or --log")
			}

			if opts.Log && opts.LogFailed {
				return cmdutil.FlagErrorf("specify only one of --log or --log-failed")
			}

			if runF != nil {
				return runF(opts)
			}
			return runView(opts)
		},
	}
	cmd.Flags().BoolVarP(&opts.Verbose, "verbose", "v", false, "Show job steps")
	// TODO should we try and expose pending via another exit code?
	cmd.Flags().BoolVar(&opts.ExitStatus, "exit-status", false, "Exit with non-zero status if run failed")
	cmd.Flags().StringVarP(&opts.JobID, "job", "j", "", "View a specific job ID from a run")
	cmd.Flags().BoolVar(&opts.Log, "log", false, "View full log for either a run or specific job")
	cmd.Flags().BoolVar(&opts.LogFailed, "log-failed", false, "View the log for any failed steps in a run or specific job")
	cmd.Flags().BoolVarP(&opts.Web, "web", "w", false, "Open run in the browser")
	cmd.Flags().Uint64VarP(&opts.Attempt, "attempt", "a", 0, "The attempt number of the workflow run")
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, shared.SingleRunFields)

	return cmd
}

func runView(opts *ViewOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return fmt.Errorf("failed to create http client: %w", err)
	}
	client := api.NewClientFromHTTP(httpClient)

	repo, err := opts.BaseRepo()
	if err != nil {
		return fmt.Errorf("failed to determine base repo: %w", err)
	}

	jobID := opts.JobID
	runID := opts.RunID
	attempt := opts.Attempt
	var selectedJob *shared.Job
	var run *shared.Run
	var jobs []shared.Job

	defer opts.IO.StopProgressIndicator()

	if jobID != "" {
		opts.IO.StartProgressIndicator()
		selectedJob, err = shared.GetJob(client, repo, jobID)
		opts.IO.StopProgressIndicator()
		if err != nil {
			return fmt.Errorf("failed to get job: %w", err)
		}
		// TODO once more stuff is merged, standardize on using ints
		runID = fmt.Sprintf("%d", selectedJob.RunID)
	}

	cs := opts.IO.ColorScheme()

	if opts.Prompt {
		// TODO arbitrary limit
		opts.IO.StartProgressIndicator()
		runs, err := shared.GetRuns(client, repo, nil, 10)
		opts.IO.StopProgressIndicator()
		if err != nil {
			return fmt.Errorf("failed to get runs: %w", err)
		}
		runID, err = shared.SelectRun(opts.Prompter, cs, runs.WorkflowRuns)
		if err != nil {
			return err
		}
	}

	opts.IO.StartProgressIndicator()
	run, err = shared.GetRun(client, repo, runID, attempt)
	opts.IO.StopProgressIndicator()
	if err != nil {
		return fmt.Errorf("failed to get run: %w", err)
	}

	if shouldFetchJobs(opts) {
		opts.IO.StartProgressIndicator()
		jobs, err = shared.GetJobs(client, repo, run, attempt)
		opts.IO.StopProgressIndicator()
		if err != nil {
			return err
		}
	}

	if opts.Prompt && len(jobs) > 1 {
		selectedJob, err = promptForJob(opts.Prompter, cs, jobs)
		if err != nil {
			return err
		}
	}

	if err := opts.IO.StartPager(); err == nil {
		defer opts.IO.StopPager()
	} else {
		fmt.Fprintf(opts.IO.ErrOut, "failed to start pager: %v\n", err)
	}

	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, run)
	}

	if opts.Web {
		url := run.URL
		if selectedJob != nil {
			url = selectedJob.URL + "?check_suite_focus=true"
		}
		if opts.IO.IsStdoutTTY() {
			fmt.Fprintf(opts.IO.Out, "Opening %s in your browser.\n", text.DisplayURL(url))
		}

		return opts.Browser.Browse(url)
	}

	if selectedJob == nil && len(jobs) == 0 {
		opts.IO.StartProgressIndicator()
		jobs, err = shared.GetJobs(client, repo, run, attempt)
		opts.IO.StopProgressIndicator()
		if err != nil {
			return fmt.Errorf("failed to get jobs: %w", err)
		}
	} else if selectedJob != nil {
		jobs = []shared.Job{*selectedJob}
	}

	if opts.Log || opts.LogFailed {
		if selectedJob != nil && selectedJob.Status != shared.Completed {
			return fmt.Errorf("job %d is still in progress; logs will be available when it is complete", selectedJob.ID)
		}

		if run.Status != shared.Completed {
			return fmt.Errorf("run %d is still in progress; logs will be available when it is complete", run.ID)
		}

		opts.IO.StartProgressIndicator()
		runLogZip, err := getRunLog(opts.RunLogCache, httpClient, repo, run, attempt)
		opts.IO.StopProgressIndicator()
		if err != nil {
			return fmt.Errorf("failed to get run log: %w", err)
		}
		defer runLogZip.Close()

		zlm := getZipLogMap(&runLogZip.Reader, jobs)
		segments, err := populateLogSegments(httpClient, repo, jobs, zlm, opts.LogFailed)
		if err != nil {
			if errors.Is(err, errTooManyAPILogFetchers) {
				return fmt.Errorf("too many API requests needed to fetch logs; try narrowing down to a specific job with the `--job` option")
			}
			return err
		}

		return displayLogSegments(opts.IO.Out, segments)
	}

	prNumber := ""
	number, err := shared.PullRequestForRun(client, repo, *run)
	if err == nil {
		prNumber = fmt.Sprintf(" %s#%d", ghrepo.FullName(repo), number)
	}

	var artifacts []shared.Artifact
	if selectedJob == nil {
		artifacts, err = shared.ListArtifacts(httpClient, repo, strconv.FormatInt(int64(run.ID), 10))
		if err != nil {
			return fmt.Errorf("failed to get artifacts: %w", err)
		}
	}

	var annotations []shared.Annotation
	var missingAnnotationsPermissions bool

	for _, job := range jobs {
		as, err := shared.GetAnnotations(client, repo, job)
		if err != nil {
			if err != shared.ErrMissingAnnotationsPermissions {
				return fmt.Errorf("failed to get annotations: %w", err)
			}

			missingAnnotationsPermissions = true
			break
		}
		annotations = append(annotations, as...)
	}

	out := opts.IO.Out

	fmt.Fprintln(out)
	fmt.Fprintln(out, shared.RenderRunHeader(cs, *run, text.FuzzyAgo(opts.Now(), run.StartedTime()), prNumber, attempt))
	fmt.Fprintln(out)

	if len(jobs) == 0 && run.Conclusion == shared.Failure || run.Conclusion == shared.StartupFailure {
		fmt.Fprintf(out, "%s %s\n",
			cs.FailureIcon(),
			cs.Bold("This run likely failed because of a workflow file issue."))

		fmt.Fprintln(out)
		fmt.Fprintf(out, "For more information, see: %s\n", cs.Bold(run.URL))

		if opts.ExitStatus {
			return cmdutil.SilentError
		}
		return nil
	}

	if selectedJob == nil {
		fmt.Fprintln(out, cs.Bold("JOBS"))
		fmt.Fprintln(out, shared.RenderJobs(cs, jobs, opts.Verbose))
	} else {
		fmt.Fprintln(out, shared.RenderJobs(cs, jobs, true))
	}

	if missingAnnotationsPermissions {
		fmt.Fprintln(out)
		fmt.Fprintln(out, cs.Bold("ANNOTATIONS"))
		fmt.Fprintln(out, "requesting annotations returned 403 Forbidden as the token does not have sufficient permissions. Note that it is not currently possible to create a fine-grained PAT with the `checks:read` permission.")
	} else if len(annotations) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, cs.Bold("ANNOTATIONS"))
		fmt.Fprintln(out, shared.RenderAnnotations(cs, annotations))
	}

	if selectedJob == nil {
		if len(artifacts) > 0 {
			fmt.Fprintln(out)
			fmt.Fprintln(out, cs.Bold("ARTIFACTS"))
			for _, a := range artifacts {
				expiredBadge := ""
				if a.Expired {
					expiredBadge = cs.Muted(" (expired)")
				}
				fmt.Fprintf(out, "%s%s\n", a.Name, expiredBadge)
			}
		}

		fmt.Fprintln(out)
		if shared.IsFailureState(run.Conclusion) {
			fmt.Fprintf(out, "To see what failed, try: gh run view %d --log-failed\n", run.ID)
		} else if len(jobs) == 1 {
			fmt.Fprintf(out, "For more information about the job, try: gh run view --job=%d\n", jobs[0].ID)
		} else {
			fmt.Fprintf(out, "For more information about a job, try: gh run view --job=<job-id>\n")
		}
		fmt.Fprintln(out, cs.Mutedf("View this run on GitHub: %s", run.URL))

		if opts.ExitStatus && shared.IsFailureState(run.Conclusion) {
			return cmdutil.SilentError
		}
	} else {
		fmt.Fprintln(out)
		if shared.IsFailureState(selectedJob.Conclusion) {
			fmt.Fprintf(out, "To see the logs for the failed steps, try: gh run view --log-failed --job=%d\n", selectedJob.ID)
		} else {
			fmt.Fprintf(out, "To see the full job log, try: gh run view --log --job=%d\n", selectedJob.ID)
		}
		fmt.Fprintln(out, cs.Mutedf("View this run on GitHub: %s", run.URL))

		if opts.ExitStatus && shared.IsFailureState(selectedJob.Conclusion) {
			return cmdutil.SilentError
		}
	}

	return nil
}

func shouldFetchJobs(opts *ViewOptions) bool {
	if opts.Prompt {
		return true
	}
	if opts.Exporter != nil {
		for _, f := range opts.Exporter.Fields() {
			if f == "jobs" {
				return true
			}
		}
	}
	return false
}

func getLog(httpClient *http.Client, logURL string) (io.ReadCloser, error) {
	req, err := http.NewRequest("GET", logURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == 404 {
		return nil, errors.New("log not found")
	} else if resp.StatusCode != 200 {
		return nil, api.HandleHTTPError(resp)
	}

	return resp.Body, nil
}

func getRunLog(cache RunLogCache, httpClient *http.Client, repo ghrepo.Interface, run *shared.Run, attempt uint64) (*zip.ReadCloser, error) {
	cacheKey := fmt.Sprintf("%d-%d", run.ID, run.StartedTime().Unix())
	isCached, err := cache.Exists(cacheKey)
	if err != nil {
		return nil, err
	}

	if !isCached {
		// Run log does not exist in cache so retrieve and store it
		logURL := fmt.Sprintf("%srepos/%s/actions/runs/%d/logs",
			ghinstance.RESTPrefix(repo.RepoHost()), ghrepo.FullName(repo), run.ID)

		if attempt > 0 {
			logURL = fmt.Sprintf("%srepos/%s/actions/runs/%d/attempts/%d/logs",
				ghinstance.RESTPrefix(repo.RepoHost()), ghrepo.FullName(repo), run.ID, attempt)
		}

		resp, err := getLog(httpClient, logURL)
		if err != nil {
			return nil, err
		}
		defer resp.Close()

		data, err := io.ReadAll(resp)
		if err != nil {
			return nil, err
		}
		respReader := bytes.NewReader(data)

		// Check if the response is a valid zip format
		_, err = zip.NewReader(respReader, respReader.Size())
		if err != nil {
			return nil, err
		}

		err = cache.Create(cacheKey, respReader)
		if err != nil {
			return nil, err
		}
	}

	return cache.Open(cacheKey)
}

func promptForJob(prompter shared.Prompter, cs *iostreams.ColorScheme, jobs []shared.Job) (*shared.Job, error) {
	candidates := []string{"View all jobs in this run"}
	for _, job := range jobs {
		symbol, _ := shared.Symbol(cs, job.Status, job.Conclusion)
		candidates = append(candidates, fmt.Sprintf("%s %s", symbol, job.Name))
	}

	selected, err := prompter.Select("View a specific job in this run?", "", candidates)
	if err != nil {
		return nil, err
	}

	if selected > 0 {
		return &jobs[selected-1], nil
	}

	// User wants to see all jobs
	return nil, nil
}

func displayLogSegments(w io.Writer, segments []logSegment) error {
	for _, segment := range segments {
		stepName := "UNKNOWN STEP"
		if segment.step != nil {
			stepName = segment.step.Name
		}

		rc, err := segment.fetcher.GetLog()
		if err != nil {
			return err
		}

		err = func() error {
			defer rc.Close()
			prefix := fmt.Sprintf("%s\t%s\t", segment.job.Name, stepName)
			if err := copyLogWithLinePrefix(w, rc, prefix); err != nil {
				return err
			}
			return nil
		}()

		if err != nil {
			return err
		}
	}

	return nil
}

func copyLogWithLinePrefix(w io.Writer, r io.Reader, prefix string) error {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		fmt.Fprintf(w, "%s%s\n", prefix, scanner.Text())
	}
	return nil
}
