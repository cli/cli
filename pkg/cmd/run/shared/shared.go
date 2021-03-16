package shared

import (
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
	ID             int
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
	ID          int
	Status      Status
	Conclusion  Conclusion
	Name        string
	Steps       []Step
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
	URL         string    `json:"html_url"`
}

type Step struct {
	Name       string
	Status     Status
	Conclusion Conclusion
	Number     int
}

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
	ID int
}

func GetAnnotations(client *api.Client, repo ghrepo.Interface, job Job) ([]Annotation, error) {
	var result []*Annotation

	path := fmt.Sprintf("repos/%s/check-runs/%d/annotations", ghrepo.FullName(repo), job.ID)

	err := client.REST(repo.RepoHost(), "GET", path, nil, &result)
	if err != nil {
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
	WorkflowRuns []Run `json:"workflow_runs"`
}

func GetRuns(client *api.Client, repo ghrepo.Interface, limit int) ([]Run, error) {
	perPage := limit
	page := 1
	if limit > 100 {
		perPage = 100
	}

	runs := []Run{}

	for len(runs) < limit {
		var result RunsPayload

		path := fmt.Sprintf("repos/%s/actions/runs?per_page=%d&page=%d", ghrepo.FullName(repo), perPage, page)

		err := client.REST(repo.RepoHost(), "GET", path, nil, &result)
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
	parsed, err := url.Parse(run.JobsURL)
	if err != nil {
		return nil, err
	}

	err = client.REST(repo.RepoHost(), "GET", parsed.Path[1:], nil, &result)
	if err != nil {
		return nil, err
	}
	return result.Jobs, nil
}

func PromptForRun(cs *iostreams.ColorScheme, client *api.Client, repo ghrepo.Interface) (string, error) {
	// TODO arbitrary limit
	runs, err := GetRuns(client, repo, 10)
	if err != nil {
		return "", err
	}

	var selected int

	candidates := []string{}

	for _, run := range runs {
		symbol, _ := Symbol(cs, run.Status, run.Conclusion)
		candidates = append(candidates,
			fmt.Sprintf("%s %s, %s (%s)", symbol, run.CommitMsg(), run.Name, run.HeadBranch))
	}

	// TODO consider custom filter so it's fuzzier. right now matches start anywhere in string but
	// become contiguous
	err = prompt.SurveyAskOne(&survey.Select{
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
