package shared

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
	workflowShared "github.com/cli/cli/v2/pkg/cmd/workflow/shared"
	"github.com/cli/cli/v2/pkg/iostreams"
)

type Prompter interface {
	Select(string, string, []string) (int, error)
}

const (
	// Run statuses
	Queued     Status = "queued"
	Completed  Status = "completed"
	InProgress Status = "in_progress"
	Requested  Status = "requested"
	Waiting    Status = "waiting"
	Pending    Status = "pending"

	// Run conclusions
	ActionRequired Conclusion = "action_required"
	Cancelled      Conclusion = "cancelled"
	Failure        Conclusion = "failure"
	Neutral        Conclusion = "neutral"
	Skipped        Conclusion = "skipped"
	Stale          Conclusion = "stale"
	StartupFailure Conclusion = "startup_failure"
	Success        Conclusion = "success"
	TimedOut       Conclusion = "timed_out"

	AnnotationFailure Level = "failure"
	AnnotationWarning Level = "warning"
)

type Status string
type Conclusion string
type Level string

var AllStatuses = []string{
	"queued",
	"completed",
	"in_progress",
	"requested",
	"waiting",
	"pending",
	"action_required",
	"cancelled",
	"failure",
	"neutral",
	"skipped",
	"stale",
	"startup_failure",
	"success",
	"timed_out",
}

var RunFields = []string{
	"name",
	"displayTitle",
	"headBranch",
	"headSha",
	"createdAt",
	"updatedAt",
	"startedAt",
	"attempt",
	"status",
	"conclusion",
	"event",
	"number",
	"databaseId",
	"workflowDatabaseId",
	"workflowName",
	"url",
}

var SingleRunFields = append(RunFields, "jobs")

type Run struct {
	Name           string    `json:"name"` // the semantics of this field are unclear
	DisplayTitle   string    `json:"display_title"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	StartedAt      time.Time `json:"run_started_at"`
	Status         Status
	Conclusion     Conclusion
	Event          string
	ID             int64
	workflowName   string // cache column
	WorkflowID     int64  `json:"workflow_id"`
	Number         int64  `json:"run_number"`
	Attempt        uint64 `json:"run_attempt"`
	HeadBranch     string `json:"head_branch"`
	JobsURL        string `json:"jobs_url"`
	HeadCommit     Commit `json:"head_commit"`
	HeadSha        string `json:"head_sha"`
	URL            string `json:"html_url"`
	HeadRepository Repo   `json:"head_repository"`
	Jobs           []Job  `json:"-"` // Populated manually (separate from fetching the run)
}

func (r *Run) StartedTime() time.Time {
	if r.StartedAt.IsZero() {
		return r.CreatedAt
	}
	return r.StartedAt
}

func (r *Run) Duration(now time.Time) time.Duration {
	endTime := r.UpdatedAt
	if r.Status != Completed {
		endTime = now
	}
	d := endTime.Sub(r.StartedTime())
	if d < 0 {
		return 0
	}
	return d.Round(time.Second)
}

type Repo struct {
	Owner struct {
		Login string
	}
	Name string
}

type Commit struct {
	Message string
}

// Title is the display title for a run, falling back to the commit subject if unavailable
func (r Run) Title() string {
	if r.DisplayTitle != "" {
		return r.DisplayTitle
	}

	commitLines := strings.Split(r.HeadCommit.Message, "\n")
	if len(commitLines) > 0 {
		return commitLines[0]
	} else {
		return r.HeadSha[0:8]
	}
}

// WorkflowName returns the human-readable name of the workflow that this run belongs to.
// TODO: consider lazy-loading the underlying API data to avoid extra API calls unless necessary
func (r Run) WorkflowName() string {
	return r.workflowName
}

func (r *Run) ExportData(fields []string) map[string]any {
	v := reflect.ValueOf(r).Elem()
	fieldByName := func(v reflect.Value, field string) reflect.Value {
		return v.FieldByNameFunc(func(s string) bool {
			return strings.EqualFold(field, s)
		})
	}
	data := map[string]any{}

	for _, f := range fields {
		switch f {
		case "databaseId":
			data[f] = r.ID
		case "workflowDatabaseId":
			data[f] = r.WorkflowID
		case "workflowName":
			data[f] = r.WorkflowName()
		case "jobs":
			jobs := make([]any, 0, len(r.Jobs))
			for _, j := range r.Jobs {
				steps := make([]any, 0, len(j.Steps))
				for _, s := range j.Steps {
					var stepCompletedAt time.Time
					if !s.CompletedAt.IsZero() {
						stepCompletedAt = s.CompletedAt
					}
					steps = append(steps, map[string]any{
						"name":        s.Name,
						"status":      s.Status,
						"conclusion":  s.Conclusion,
						"number":      s.Number,
						"startedAt":   s.StartedAt,
						"completedAt": stepCompletedAt,
					})
				}
				var jobCompletedAt time.Time
				if !j.CompletedAt.IsZero() {
					jobCompletedAt = j.CompletedAt
				}
				jobs = append(jobs, map[string]any{
					"databaseId":  j.ID,
					"status":      j.Status,
					"conclusion":  j.Conclusion,
					"name":        j.Name,
					"steps":       steps,
					"startedAt":   j.StartedAt,
					"completedAt": jobCompletedAt,
					"url":         j.URL,
				})
			}
			data[f] = jobs
		default:
			sf := fieldByName(v, f)
			data[f] = sf.Interface()
		}
	}

	return data
}

type Job struct {
	ID          int64
	Status      Status
	Conclusion  Conclusion
	Name        string
	Steps       Steps
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
	URL         string    `json:"html_url"`
	RunID       int64     `json:"run_id"`
}

type Step struct {
	Name        string
	Status      Status
	Conclusion  Conclusion
	Number      int
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
}

type Steps []Step

func (s Steps) Len() int           { return len(s) }
func (s Steps) Less(i, j int) bool { return s[i].Number < s[j].Number }
func (s Steps) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

type Annotation struct {
	JobName   string
	Message   string
	Path      string
	Level     Level `json:"annotation_level"`
	StartLine int   `json:"start_line"`
}

func AnnotationSymbol(cs *iostreams.ColorScheme, a Annotation) string {
	switch a.Level {
	case AnnotationFailure:
		return cs.FailureIcon()
	case AnnotationWarning:
		return cs.WarningIcon()
	default:
		return "-"
	}
}

type CheckRun struct {
	ID int64
}

var ErrMissingAnnotationsPermissions = errors.New("missing annotations permissions error")

// GetAnnotations fetches annotations from the REST API.
//
// If the job has no annotations, an empty slice is returned.
// If the API returns a 403, a custom ErrMissingAnnotationsPermissions error is returned.
//
// When fine-grained PATs support checks:read permission, we can remove the need for this at the call sites.
func GetAnnotations(client *api.Client, repo ghrepo.Interface, job Job) ([]Annotation, error) {
	var result []*Annotation

	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "check-runs", strconv.FormatInt(job.ID, 10), "annotations")
	if err != nil {
		return nil, err
	}

	err = client.REST(repo.RepoHost(), "GET", path.String(), nil, &result)
	if err != nil {
		var httpError api.HTTPError
		if !errors.As(err, &httpError) {
			return nil, err
		}

		if httpError.StatusCode == http.StatusNotFound {
			return []Annotation{}, nil
		}

		if httpError.StatusCode == http.StatusForbidden {
			return nil, ErrMissingAnnotationsPermissions
		}

		return nil, err
	}

	out := []Annotation{}

	for _, annotation := range result {
		annotation.JobName = job.Name
		out = append(out, *annotation)
	}

	return out, nil
}

func IsFailureState(c Conclusion) bool {
	switch c {
	case ActionRequired, Failure, StartupFailure, TimedOut:
		return true
	default:
		return false
	}
}

func IsSkipped(c Conclusion) bool {
	return c == Skipped
}

type RunsPayload struct {
	TotalCount   int   `json:"total_count"`
	WorkflowRuns []Run `json:"workflow_runs"`
}

type FilterOptions struct {
	Branch     string
	Actor      string
	WorkflowID int64
	// avoid loading workflow name separately and use the provided one
	WorkflowName string
	Status       string
	Event        string
	Created      string
	Commit       string
}

// GetRunsWithFilter fetches 50 runs from the API and filters them in-memory
func GetRunsWithFilter(client *api.Client, repo ghrepo.Interface, opts *FilterOptions, limit int, f func(Run) bool) ([]Run, error) {
	runs, err := GetRuns(client, repo, opts, 50)
	if err != nil {
		return nil, err
	}

	var filtered []Run
	for _, run := range runs.WorkflowRuns {
		if f(run) {
			filtered = append(filtered, run)
		}
		if len(filtered) == limit {
			break
		}
	}

	return filtered, nil
}

func GetRuns(client *api.Client, repo ghrepo.Interface, opts *FilterOptions, limit int) (*RunsPayload, error) {
	u, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "actions", "runs")
	if err != nil {
		return nil, err
	}
	if opts != nil && opts.WorkflowID > 0 {
		u, err = safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "actions", "workflows", strconv.FormatInt(opts.WorkflowID, 10), "runs")
		if err != nil {
			return nil, err
		}
	}

	perPage := min(limit, 100)
	u.SetQuery("per_page", strconv.Itoa(perPage))
	u.SetQuery("exclude_pull_requests", "true") // significantly reduces payload size

	if opts != nil {
		if opts.Branch != "" {
			u.SetQuery("branch", opts.Branch)
		}
		if opts.Actor != "" {
			u.SetQuery("actor", opts.Actor)
		}
		if opts.Status != "" {
			u.SetQuery("status", opts.Status)
		}
		if opts.Event != "" {
			u.SetQuery("event", opts.Event)
		}
		if opts.Created != "" {
			u.SetQuery("created", opts.Created)
		}
		if opts.Commit != "" {
			u.SetQuery("head_sha", opts.Commit)
		}
	}
	var pageURL safeurl.SafeURL = u

	var result *RunsPayload

pagination:
	for pageURL.String() != "" {
		var response RunsPayload
		next, err := client.RESTWithNext(repo.RepoHost(), "GET", pageURL.String(), nil, &response)
		if err != nil {
			return nil, err
		}
		pageURL = safeurl.NewImmutableSafeURL(next)

		if result == nil {
			result = &response
			if len(result.WorkflowRuns) == limit {
				break pagination
			}
		} else {
			for _, run := range response.WorkflowRuns {
				result.WorkflowRuns = append(result.WorkflowRuns, run)
				if len(result.WorkflowRuns) == limit {
					break pagination
				}
			}
		}
	}

	if opts != nil && opts.WorkflowName != "" {
		for i := range result.WorkflowRuns {
			result.WorkflowRuns[i].workflowName = opts.WorkflowName
		}
	} else if len(result.WorkflowRuns) > 0 {
		if err := preloadWorkflowNames(client, repo, result.WorkflowRuns); err != nil {
			return result, err
		}
	}

	return result, nil
}

func preloadWorkflowNames(client *api.Client, repo ghrepo.Interface, runs []Run) error {
	workflows, err := workflowShared.GetWorkflows(client, repo, 0)
	if err != nil {
		return err
	}

	workflowMap := map[int64]string{}
	for _, wf := range workflows {
		workflowMap[wf.ID] = wf.Name
	}

	for i, run := range runs {
		if _, ok := workflowMap[run.WorkflowID]; !ok {
			// Look up workflow by ID because it may have been deleted
			workflow, err := workflowShared.GetWorkflow(client, repo, run.WorkflowID)
			// If the error is an httpError and it is a 404, this is likely a
			// organization or enterprise ruleset workflow. The user does not
			// have permissions to view the details of the workflow, so we cannot
			// look it up directly without receiving a 404, but it is nonetheless
			// in the workflow run list. To handle this, we set the workflow name
			// to an empty string.
			// Deciding to put this here instead of in GetWorkflow to allow
			// the caller to decide what a 404 means.
			if httpErr, ok := err.(api.HTTPError); ok && httpErr.StatusCode == 404 {
				workflowMap[run.WorkflowID] = ""
				continue
			}
			if err != nil {
				return err
			}
			workflowMap[run.WorkflowID] = workflow.Name
		}
		runs[i].workflowName = workflowMap[run.WorkflowID]
	}
	return nil
}

type JobsPayload struct {
	TotalCount int `json:"total_count"`
	Jobs       []Job
}

func GetJobs(client *api.Client, repo ghrepo.Interface, runID int64, jobsURL safeurl.SafeURL, attempt uint64) ([]Job, error) {
	var jobsPath safeurl.SafeURL
	if attempt > 0 {
		p, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "actions", "runs", strconv.FormatInt(runID, 10), "attempts", strconv.FormatUint(attempt, 10), "jobs")
		if err != nil {
			return nil, err
		}
		p.SetQuery("per_page", "100")
		jobsPath = p
	} else {
		u, err := url.Parse(jobsURL.String())
		if err != nil {
			return nil, err
		}
		query := url.Values{}
		query.Set("per_page", "100")
		u.RawQuery = query.Encode()
		// Since u is derived from jobsURL, an already-trusted safeurl.SafeURL, the resulting URL is safe to declare as such.
		jobsPath = safeurl.NewImmutableSafeURL(u.String())
	}

	// A non-nil empty slice is returned so callers can tell that jobs were fetched and there are none (if len is zero).
	jobs := []Job{}
	for jobsPath.String() != "" {
		var resp JobsPayload
		next, err := client.RESTWithNext(repo.RepoHost(), http.MethodGet, jobsPath.String(), nil, &resp)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, resp.Jobs...)
		jobsPath = safeurl.NewImmutableSafeURL(next)
	}
	return jobs, nil
}

func GetJob(client *api.Client, repo ghrepo.Interface, jobID string) (*Job, error) {
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "actions", "jobs", jobID)
	if err != nil {
		return nil, err
	}

	var result Job
	err = client.REST(repo.RepoHost(), "GET", path.String(), nil, &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// SelectRun prompts the user to select a run from a list of runs by using the recommended prompter interface
func SelectRun(p Prompter, cs *iostreams.ColorScheme, runs []Run) (string, error) {
	now := time.Now()

	candidates := []string{}

	for _, run := range runs {
		symbol, _ := Symbol(cs, run.Status, run.Conclusion)
		candidates = append(candidates,
			// TODO truncate commit message, long ones look terrible
			fmt.Sprintf("%s %s, %s [%s] %s", symbol, run.Title(), run.WorkflowName(), run.HeadBranch, preciseAgo(now, run.StartedTime())))
	}

	selected, err := p.Select("Select a workflow run", "", candidates)

	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%d", runs[selected].ID), nil
}

func GetRun(client *api.Client, repo ghrepo.Interface, runID string, attempt uint64) (*Run, error) {
	var result Run

	u, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "actions", "runs", runID)
	if err != nil {
		return nil, err
	}

	if attempt > 0 {
		u, err = safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "actions", "runs", runID, "attempts", strconv.FormatUint(attempt, 10))
		if err != nil {
			return nil, err
		}
	}
	u.SetQuery("exclude_pull_requests", "true")

	err = client.REST(repo.RepoHost(), "GET", u.String(), nil, &result)
	if err != nil {
		return nil, err
	}

	if attempt > 0 {
		result.URL, err = url.JoinPath(result.URL, fmt.Sprintf("/attempts/%d", attempt))
		if err != nil {
			return nil, err
		}
	}

	// Set name to workflow name
	workflow, err := workflowShared.GetWorkflow(client, repo, result.WorkflowID)
	if err != nil {
		return nil, err
	} else {
		result.workflowName = workflow.Name
	}

	return &result, nil
}

type colorFunc func(string) string

func Symbol(cs *iostreams.ColorScheme, status Status, conclusion Conclusion) (string, colorFunc) {
	noColor := func(s string) string { return s }
	if status == Completed {
		switch conclusion {
		case Success:
			return cs.SuccessIconWithColor(noColor), cs.Green
		case Skipped, Neutral:
			return "-", cs.Muted
		default:
			return cs.FailureIconWithColor(noColor), cs.Red
		}
	}

	return "*", cs.Yellow
}

func PullRequestForRun(client *api.Client, repo ghrepo.Interface, run Run) (int, error) {
	type response struct {
		Repository struct {
			PullRequests struct {
				Nodes []struct {
					Number         int
					HeadRepository struct {
						Owner struct {
							Login string
						}
						Name string
					}
				}
			}
		}
		Number int
	}

	variables := map[string]any{
		"owner":       repo.RepoOwner(),
		"repo":        repo.RepoName(),
		"headRefName": run.HeadBranch,
	}

	query := `
		query PullRequestForRun($owner: String!, $repo: String!, $headRefName: String!) {
			repository(owner: $owner, name: $repo) {
				pullRequests(headRefName: $headRefName, first: 1, orderBy: { field: CREATED_AT, direction: DESC }) {
					nodes {
						number
						headRepository {
							owner {
								login
							}
							name
						}
					}
				}
			}
		}`

	var resp response

	err := client.GraphQL(repo.RepoHost(), query, variables, &resp)
	if err != nil {
		return -1, err
	}

	prs := resp.Repository.PullRequests.Nodes
	if len(prs) == 0 {
		return -1, fmt.Errorf("no matching PR found for %s", run.HeadBranch)
	}

	number := -1

	for _, pr := range prs {
		if pr.HeadRepository.Owner.Login == run.HeadRepository.Owner.Login && pr.HeadRepository.Name == run.HeadRepository.Name {
			number = pr.Number
		}
	}

	if number == -1 {
		return number, fmt.Errorf("no matching PR found for %s", run.HeadBranch)
	}

	return number, nil
}

func preciseAgo(now time.Time, createdAt time.Time) string {
	ago := now.Sub(createdAt)

	if ago < 30*24*time.Hour {
		s := ago.Truncate(time.Second).String()
		return fmt.Sprintf("%s ago", s)
	}

	return createdAt.Format("Jan _2, 2006")
}
