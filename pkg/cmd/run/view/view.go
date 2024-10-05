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
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

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

			This command does not support authenticating via fine grained PATs
			as it is not currently possible to create a PAT with the %[1]schecks:read%[1]s permission.
		`, "`"),
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

		attachRunLog(&runLogZip.Reader, jobs)

		return displayRunLog(opts.IO.Out, jobs, opts.LogFailed)
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
					expiredBadge = cs.Gray(" (expired)")
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

const JOB_NAME_MAX_LENGTH = 90

func logFilenameRegexp(job shared.Job, step shared.Step) *regexp.Regexp {
	// As described in https://github.com/cli/cli/issues/5011#issuecomment-1570713070, there are a number of steps
	// the server can take when producing the downloaded zip file that can result in a mismatch between the job name
	// and the filename in the zip including:
	//  * Removing characters in the job name that aren't allowed in file paths
	//  * Truncating names that are too long for zip files
	//  * Adding collision deduplicating numbers for jobs with the same name
	//
	// We are hesitant to duplicate all the server logic due to the fragility but it may be unavoidable. Currently, we:
	// * Strip `/` which occur when composite action job names are constructed of the form `<JOB_NAME`> / <ACTION_NAME>`
	// * Truncate long job names
	//
	sanitizedJobName := strings.ReplaceAll(job.Name, "/", "")
	sanitizedJobName = strings.ReplaceAll(sanitizedJobName, ":", "")
	sanitizedJobName = truncateAsUTF16(sanitizedJobName, JOB_NAME_MAX_LENGTH)
	re := fmt.Sprintf(`^%s\/%d_.*\.txt`, regexp.QuoteMeta(sanitizedJobName), step.Number)
	return regexp.MustCompile(re)
}

/*
If you're reading this comment by necessity, I'm sorry and if you're reading it for fun, you're welcome, you weirdo.

What is the length of this string "a😅😅"? If you said 9 you'd be right. If you said 3 or 5 you might also be right!

Here's a summary:

	"a" takes 1 byte (`\x61`)
	"😅" takes 4 `bytes` (`\xF0\x9F\x98\x85`)
	"a😅😅" therefore takes 9 `bytes`
	In Go `len("a😅😅")` is 9 because the `len` builtin counts `bytes`
	In Go `len([]rune("a😅😅"))` is 3 because each `rune` is 4 `bytes` so each character fits within a `rune`
	In C# `"a😅😅".Length` is 5 because `.Length` counts `Char` objects, `Chars` hold 2 bytes, and "😅" takes 2 Chars.

But wait, what does C# have to do with anything? Well the server is running C#. Which server? The one that serves log
files to us in `.zip` format of course! When the server is constructing the zip file to avoid running afoul of a 260
byte zip file path length limitation, it applies transformations to various strings in order to limit their length.
In C#, the server truncates strings with this function:

	public static string TruncateAfter(string str, int max)
	{
		string result = str.Length > max ? str.Substring(0, max) : str;
		result = result.Trim();
		return result;
	}

This seems like it would be easy enough to replicate in Go but as we already discovered, the length of a string isn't
as obvious as it might seem. Since C# uses UTF-16 encoding for strings, and Go uses UTF-8 encoding and represents
characters by runes (which are an alias of int32) we cannot simply slice the string without any further consideration.
Instead, we need to encode the string as UTF-16 bytes, slice it and then decode it back to UTF-8.

Interestingly, in C# length and substring both act on the Char type so it's possible to slice into the middle of
a visual, "representable" character. For example we know `"a😅😅".Length` = 5 (1+2+2) and therefore Substring(0,4)
results in the final character being cleaved in two, resulting in "a😅�". Since our int32 runes are being encoded as
2 uint16 elements, we also mimic this behaviour by slicing into the UTF-16 encoded string.

Here's a program you can put into a dotnet playground to see how C# works:

	using System;
	public class Program {
	  public static void Main() {
	    string s = "a😅😅";
	    Console.WriteLine("{0} {1}", s.Length, s);
	    string t = TruncateAfter(s, 4);
	    Console.WriteLine("{0} {1}", t.Length, t);
	  }
	  public static string TruncateAfter(string str, int max) {
	    string result = str.Length > max ? str.Substring(0, max) : str;
	    return result.Trim();
	  }
	}

This will output:
5 a😅😅
4 a😅�
*/
func truncateAsUTF16(str string, max int) string {
	// Encode the string to UTF-16 to count code units
	utf16Encoded := utf16.Encode([]rune(str))
	if len(utf16Encoded) > max {
		// Decode back to UTF-8 up to the max length
		str = string(utf16.Decode(utf16Encoded[:max]))
	}
	return strings.TrimSpace(str)
}

// This function takes a zip file of logs and a list of jobs.
// Structure of zip file
//
//	zip/
//	├── jobname1/
//	│   ├── 1_stepname.txt
//	│   ├── 2_anotherstepname.txt
//	│   ├── 3_stepstepname.txt
//	│   └── 4_laststepname.txt
//	└── jobname2/
//	    ├── 1_stepname.txt
//	    └── 2_somestepname.txt
//
// It iterates through the list of jobs and tries to find the matching
// log in the zip file. If the matching log is found it is attached
// to the job.
func attachRunLog(rlz *zip.Reader, jobs []shared.Job) {
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

func displayRunLog(w io.Writer, jobs []shared.Job, failed bool) error {
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
				fmt.Fprintf(w, "%s%s\n", prefix, scanner.Text())
			}
			f.Close()
		}
	}

	return nil
}
