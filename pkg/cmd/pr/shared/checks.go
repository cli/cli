package shared

import (
	"fmt"
	"time"

	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/utils"
)

type CheckRunPayload struct {
	Name        string
	Status      string
	Conclusion  string
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
	HtmlUrl     string    `json:"html_url"`
}

type CheckRunsPayload struct {
	CheckRuns []CheckRunPayload `json:"check_runs"`
}

type CheckRun struct {
	Name    string
	Status  string
	Link    string
	Elapsed time.Duration
}

type CheckRunList struct {
	Passing   int
	Failing   int
	Pending   int
	CheckRuns []CheckRun
}

func CheckRuns(client *api.Client, repo ghrepo.Interface, pr *api.PullRequest) (*CheckRunList, error) {
	list := CheckRunList{}

	if len(pr.Commits.Nodes) == 0 {
		return &list, nil
	}

	path := fmt.Sprintf("repos/%s/%s/commits/%s/check-runs",
		repo.RepoOwner(), repo.RepoName(), pr.Commits.Nodes[0].Commit.Oid)
	var response CheckRunsPayload

	err := client.REST(repo.RepoHost(), "GET", path, nil, &response)
	if err != nil {
		return nil, err
	}

	for _, cr := range response.CheckRuns {
		elapsed := cr.CompletedAt.Sub(cr.StartedAt)

		run := CheckRun{
			Elapsed: elapsed,
			Name:    cr.Name,
			Link:    cr.HtmlUrl,
		}

		if cr.Status == "in_progress" || cr.Status == "queued" {
			list.Pending++
			run.Status = "pending"
		} else if cr.Status == "completed" {
			// TODO stale?
			switch cr.Conclusion {
			case "neutral", "success", "skipped":
				list.Passing++
				run.Status = "pass"
			case "cancelled", "timed_out", "failure", "action_required":
				list.Failing++
				run.Status = "fail"
			}
		} else {
			panic(fmt.Errorf("unsupported status: %q", cr.Status))
		}

		list.CheckRuns = append(list.CheckRuns, run)
	}

	return &list, nil
}

func (runList *CheckRunList) Summary() string {
	fails := runList.Failing
	passes := runList.Passing
	pending := runList.Pending

	if fails+passes+pending == 0 {
		return ""
	}

	summary := ""

	if runList.Failing > 0 {
		summary = "Some checks were not successful"
	} else if runList.Pending > 0 {
		summary = "Some checks are still pending"
	} else {
		summary = "All checks were successful"
	}

	tallies := fmt.Sprintf(
		"%d failing, %d successful, and %d pending checks",
		fails, passes, pending)

	return fmt.Sprintf("%s\n%s", utils.Bold(summary), tallies)
}
