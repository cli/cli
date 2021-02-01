package action

import (
	"fmt"
	"strconv"
	"time"

	"github.com/rsteube/carapace"
	"github.com/spf13/cobra"
)

type job struct {
	Id        int
	Name      string
	StartedAt time.Time `json:"started_at"`
}

type JobOpts struct {
}

type jobQuery struct {
	Jobs []job
}

func ActionWorkflowJobs(cmd *cobra.Command, runId string, opts RunOpts) carapace.Action {
	return carapace.ActionCallback(func(c carapace.Context) carapace.Action {
		repo, err := repoOverride(cmd)
		if err != nil {
			return carapace.ActionMessage(err.Error())
		}

		var queryResult jobQuery
		return ApiV3Action(cmd, fmt.Sprintf(`repos/%v/%v/actions/runs/%v/jobs`, repo.RepoOwner(), repo.RepoName(), runId), &queryResult, func() carapace.Action {
			vals := make([]string, 0)
			for _, job := range queryResult.Jobs {
				vals = append(vals, strconv.Itoa(job.Id), job.Name)
			}
			return carapace.ActionValuesDescribed(vals...)
		})
	})
}
