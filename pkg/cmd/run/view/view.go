package view

import (
	"archive/zip"
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/run/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/prompt"
	"github.com/cli/cli/v2/utils"
	"github.com/spf13/cobra"
)

type browser interface {
	Browse(string) error
}

type runLogCache interface {
	Exists(string) bool
	Create(string, io.ReadCloser) error
	Open(string) (*zip.ReadCloser, error)
}

type rlc struct{}

func (rlc) Exists(path string) bool {
	if _, err := os.Stat(path); err != nil {
		return false
	}
	return true
}
func (rlc) Create(path string, content io.ReadCloser) error {
	err := os.MkdirAll(filepath.Dir(path), 0755)
	if err != nil {
		return fmt.Errorf("could not create cache: %w", err)
	}

	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, content)
	return err
}
func (rlc) Open(path string) (*zip.ReadCloser, error) {
	return zip.OpenReader(path)
}

type ViewOptions struct {
	HttpClient  func() (*http.Client, error)
	IO          *iostreams.IOStreams
	BaseRepo    func() (ghrepo.Interface, error)
	Browser     browser
	RunLogCache runLogCache

	RunID      string
	JobID      string
	Verbose    bool
	ExitStatus bool
	Log        bool
	LogFailed  bool
	Web        bool

	Prompt bool

	Now func() time.Time
}

func NewCmdView(f *cmdutil.Factory, runF func(*ViewOptions) error) *cobra.Command {
	opts := &ViewOptions{
		IO:          f.IOStreams,
		HttpClient:  f.HttpClient,
		Now:         time.Now,
		Browser:     f.Browser,
		RunLogCache: rlc{},
	}

	cmd := &cobra.Command{
		Use:   "view [<run-id>]",
		Short: "View a summary of a workflow run",
		Args:  cobra.MaximumNArgs(1),
		Example: heredoc.Doc(`
			# Interactively select a run to view, optionally selecting a single job
			$ gh run view

			# View a specific run
			$ gh run view 12345

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

			if len(args) == 0 && opts.JobID == "" {
				if !opts.IO.CanPrompt() {
					return &cmdutil.FlagError{Err: errors.New("run or job ID required when not running interactively")}
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
				return &cmdutil.FlagError{Err: errors.New("specify only one of --web or --log")}
			}

			if opts.Log && opts.LogFailed {
				return &cmdutil.FlagError{Err: errors.New("specify only one of --log or --log-failed")}
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
	var selectedJob *shared.Job
	var run *shared.Run
	var jobs []shared.Job

	defer opts.IO.StopProgressIndicator()

	if jobID != "" {
		opts.IO.StartProgressIndicator()
		selectedJob, err = getJob(client, repo, jobID)
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
		runs, err := shared.GetRuns(client, repo, 10)
		opts.IO.StopProgressIndicator()
		if err != nil {
			return fmt.Errorf("failed to get runs: %w", err)
		}
		runID, err = shared.PromptForRun(cs, runs)
		if err != nil {
			return err
		}
	}

	opts.IO.StartProgressIndicator()
	run, err = shared.GetRun(client, repo, runID)
	opts.IO.StopProgressIndicator()
	if err != nil {
		return fmt.Errorf("failed to get run: %w", err)
	}

	if opts.Prompt {
		opts.IO.StartProgressIndicator()
		jobs, err = shared.GetJobs(client, repo, *run)
		opts.IO.StopProgressIndicator()
		if err != nil {
			return err
		}
		if len(jobs) > 1 {
			selectedJob, err = promptForJob(cs, jobs)
			if err != nil {
				return err
			}
		}
	}

	if opts.Web {
		url := run.URL
		if selectedJob != nil {
			url = selectedJob.URL + "?check_suite_focus=true"
		}
		if opts.IO.IsStdoutTTY() {
			fmt.Fprintf(opts.IO.Out, "Opening %s in your browser.\n", utils.DisplayURL(url))
		}

		return opts.Browser.Browse(url)
	}

	if selectedJob == nil && len(jobs) == 0 {
		opts.IO.StartProgressIndicator()
		jobs, err = shared.GetJobs(client, repo, *run)
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
		runLogZip, err := getRunLog(opts.RunLogCache, httpClient, repo, run)
		opts.IO.StopProgressIndicator()
		if err != nil {
			return fmt.Errorf("failed to get run log: %w", err)
		}
		defer runLogZip.Close()

		attachRunLog(runLogZip, jobs)

		return displayRunLog(opts.IO, jobs, opts.LogFailed)
	}

	prNumber := ""
	number, err := shared.PullRequestForRun(client, repo, *run)
	if err == nil {
		prNumber = fmt.Sprintf(" #%d", number)
	}

	var artifacts []shared.Artifact
	if selectedJob == nil {
		artifacts, err = shared.ListArtifacts(httpClient, repo, strconv.FormatInt(int64(run.ID), 10))
		if err != nil {
			return fmt.Errorf("failed to get artifacts: %w", err)
		}
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

	opts.IO.StopProgressIndicator()

	if annotationErr != nil {
		return fmt.Errorf("failed to get annotations: %w", annotationErr)
	}

	out := opts.IO.Out

	ago := opts.Now().Sub(run.CreatedAt)

	fmt.Fprintln(out)
	fmt.Fprintln(out, shared.RenderRunHeader(cs, *run, utils.FuzzyAgo(ago), prNumber))
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

	if len(annotations) > 0 {
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
					expiredBadge = cs.Gray(" (expired)")
				}
				fmt.Fprintf(out, "%s%s\n", a.Name, expiredBadge)
			}
		}

		fmt.Fprintln(out)
		if shared.IsFailureState(run.Conclusion) {
			fmt.Fprintf(out, "To see what failed, try: gh run view %d --log-failed\n", run.ID)
		} else {
			fmt.Fprintln(out, "For more information about a job, try: gh run view --job=<job-id>")
		}
		fmt.Fprintf(out, cs.Gray("View this run on GitHub: %s\n"), run.URL)

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
		fmt.Fprintf(out, cs.Gray("View this run on GitHub: %s\n"), run.URL)

		if opts.ExitStatus && shared.IsFailureState(selectedJob.Conclusion) {
			return cmdutil.SilentError
		}
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

func getRunLog(cache runLogCache, httpClient *http.Client, repo ghrepo.Interface, run *shared.Run) (*zip.ReadCloser, error) {
	filename := fmt.Sprintf("run-log-%d-%d.zip", run.ID, run.CreatedAt.Unix())
	filepath := filepath.Join(os.TempDir(), "gh-cli-cache", filename)
	if !cache.Exists(filepath) {
		// Run log does not exist in cache so retrieve and store it
		logURL := fmt.Sprintf("%srepos/%s/actions/runs/%d/logs",
			ghinstance.RESTPrefix(repo.RepoHost()), ghrepo.FullName(repo), run.ID)

		resp, err := getLog(httpClient, logURL)
		if err != nil {
			return nil, err
		}
		defer resp.Close()

		err = cache.Create(filepath, resp)
		if err != nil {
			return nil, err
		}
	}

	return cache.Open(filepath)
}

func promptForJob(cs *iostreams.ColorScheme, jobs []shared.Job) (*shared.Job, error) {
	candidates := []string{"View all jobs in this run"}
	for _, job := range jobs {
		symbol, _ := shared.Symbol(cs, job.Status, job.Conclusion)
		candidates = append(candidates, fmt.Sprintf("%s %s", symbol, job.Name))
	}

	var selected int
	err := prompt.SurveyAskOne(&survey.Select{
		Message:  "View a specific job in this run?",
		Options:  candidates,
		PageSize: 12,
	}, &selected)
	if err != nil {
		return nil, err
	}

	if selected > 0 {
		return &jobs[selected-1], nil
	}

	// User wants to see all jobs
	return nil, nil
}

func logFilenameRegexp(job shared.Job, step shared.Step) *regexp.Regexp {
	re := fmt.Sprintf(`%s\/%d_.*\.txt`, regexp.QuoteMeta(job.Name), step.Number)
	return regexp.MustCompile(re)
}

// This function takes a zip file of logs and a list of jobs.
// Structure of zip file
// zip/
// ├── jobname1/
// │   ├── 1_stepname.txt
// │   ├── 2_anotherstepname.txt
// │   ├── 3_stepstepname.txt
// │   └── 4_laststepname.txt
// └── jobname2/
//     ├── 1_stepname.txt
//     └── 2_somestepname.txt
// It iterates through the list of jobs and trys to find the matching
// log in the zip file. If the matching log is found it is attached
// to the job.
func attachRunLog(rlz *zip.ReadCloser, jobs []shared.Job) {
	for i, job := range jobs {
		for j, step := range job.Steps {
			re := logFilenameRegexp(job, step)
			for _, file := range rlz.File {
				if re.MatchString(file.Name) {
					jobs[i].Steps[j].Log = file
					break
				}
			}
		}
	}
}

func displayRunLog(io *iostreams.IOStreams, jobs []shared.Job, failed bool) error {
	err := io.StartPager()
	if err != nil {
		return err
	}
	defer io.StopPager()

	for _, job := range jobs {
		steps := job.Steps
		sort.Sort(steps)
		for _, step := range steps {
			if failed && !shared.IsFailureState(step.Conclusion) {
				continue
			}
			if step.Log == nil {
				continue
			}
			prefix := fmt.Sprintf("%s\t%s\t", job.Name, step.Name)
			f, err := step.Log.Open()
			if err != nil {
				return err
			}
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				fmt.Fprintf(io.Out, "%s%s\n", prefix, scanner.Text())
			}
			f.Close()
		}
	}

	return nil
}
