package shared

import (
	"archive/zip"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/prompt"
)

const (
	// Run statuses
	Queued     Status = "queued"
	Completed  Status = "completed"
	InProgress Status = "in_progress"
	Requested  Status = "requested"
	Waiting    Status = "waiting"

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

type Run struct {
	Name           string
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	Status         Status
	Conclusion     Conclusion
	Event          string
	ID             int64
	HeadBranch     string `json:"head_branch"`
	JobsURL        string `json:"jobs_url"`
	HeadCommit     Commit `json:"head_commit"`
	HeadSha        string `json:"head_sha"`
	URL            string `json:"html_url"`
	HeadRepository Repo   `json:"head_repository"`
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

func (r Run) CommitMsg() string {
	commitLines := strings.Split(r.HeadCommit.Message, "\n")
	if len(commitLines) > 0 {
		return commitLines[0]
	} else {
		return r.HeadSha[0:8]
	}
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
	Name       string
	Status     Status
	Conclusion Conclusion
	Number     int
	Log        *zip.File
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

func GetAnnotations(client *api.Client, repo ghrepo.Interface, job Job) ([]Annotation, error) {
	var result []*Annotation

	path := fmt.Sprintf("repos/%s/check-runs/%d/annotations", ghrepo.FullName(repo), job.ID)

	err := client.REST(repo.RepoHost(), "GET", path, nil, &result)
	if err != nil {
		var httpError api.HTTPError
		if errors.As(err, &httpError) && httpError.StatusCode == 404 {
			return []Annotation{}, nil
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

type RunsPayload struct {
	TotalCount   int   `json:"total_count"`
	WorkflowRuns []Run `json:"workflow_runs"`
}

func GetRunsWithFilter(client *api.Client, repo ghrepo.Interface, limit int, f func(Run) bool) ([]Run, error) {
	path := fmt.Sprintf("repos/%s/actions/runs", ghrepo.FullName(repo))
	runs, err := getRuns(client, repo, path, 50)
	if err != nil {
		return nil, err
	}
	filtered := []Run{}
	for _, run := range runs {
		if f(run) {
			filtered = append(filtered, run)
		}
		if len(filtered) == limit {
			break
		}
	}

	return filtered, nil
}

func GetRunsByWorkflow(client *api.Client, repo ghrepo.Interface, limit int, workflowID int64) ([]Run, error) {
	path := fmt.Sprintf("repos/%s/actions/workflows/%d/runs", ghrepo.FullName(repo), workflowID)
	return getRuns(client, repo, path, limit)
}

func GetRuns(client *api.Client, repo ghrepo.Interface, limit int) ([]Run, error) {
	path := fmt.Sprintf("repos/%s/actions/runs", ghrepo.FullName(repo))
	return getRuns(client, repo, path, limit)
}

func getRuns(client *api.Client, repo ghrepo.Interface, path string, limit int) ([]Run, error) {
	perPage := limit
	page := 1
	if limit > 100 {
		perPage = 100
	}

	runs := []Run{}

	for len(runs) < limit {
		var result RunsPayload

		parsed, err := url.Parse(path)
		if err != nil {
			return nil, err
		}
		query := parsed.Query()
		query.Set("per_page", fmt.Sprintf("%d", perPage))
		query.Set("page", fmt.Sprintf("%d", page))
		parsed.RawQuery = query.Encode()
		pagedPath := parsed.String()

		err = client.REST(repo.RepoHost(), "GET", pagedPath, nil, &result)
		if err != nil {
			return nil, err
		}

		if len(result.WorkflowRuns) == 0 {
			break
		}

		for _, run := range result.WorkflowRuns {
			runs = append(runs, run)
			if len(runs) == limit {
				break
			}
		}

		if len(result.WorkflowRuns) < perPage {
			break
		}

		page++
	}

	return runs, nil
}

type JobsPayload struct {
	Jobs []Job
}

func GetJobs(client *api.Client, repo ghrepo.Interface, run Run) ([]Job, error) {
	var result JobsPayload
	if err := client.REST(repo.RepoHost(), "GET", run.JobsURL, nil, &result); err != nil {
		return nil, err
	}
	return result.Jobs, nil
}

func PromptForRun(cs *iostreams.ColorScheme, runs []Run) (string, error) {
	var selected int
	now := time.Now()

	candidates := []string{}

	for _, run := range runs {
		symbol, _ := Symbol(cs, run.Status, run.Conclusion)
		candidates = append(candidates,
			// TODO truncate commit message, long ones look terrible
			fmt.Sprintf("%s %s, %s (%s) %s", symbol, run.CommitMsg(), run.Name, run.HeadBranch, preciseAgo(now, run.CreatedAt)))
	}

	// TODO consider custom filter so it's fuzzier. right now matches start anywhere in string but
	// become contiguous
	err := prompt.SurveyAskOne(&survey.Select{
		Message:  "Select a workflow run",
		Options:  candidates,
		PageSize: 10,
	}, &selected)

	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%d", runs[selected].ID), nil
}

func GetRun(client *api.Client, repo ghrepo.Interface, runID string) (*Run, error) {
	var result Run

	path := fmt.Sprintf("repos/%s/actions/runs/%s", ghrepo.FullName(repo), runID)

	err := client.REST(repo.RepoHost(), "GET", path, nil, &result)
	if err != nil {
		return nil, err
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
		case Skipped, Cancelled, Neutral:
			return cs.SuccessIconWithColor(noColor), cs.Gray
		default:
			return cs.FailureIconWithColor(noColor), cs.Red
		}
	}

	return "-", cs.Yellow
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

	variables := map[string]interface{}{
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
