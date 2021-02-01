package action

import (
	"fmt"
	"strconv"

	"github.com/rsteube/carapace"
	"github.com/spf13/cobra"
)

type workflow struct {
	Id    int
	Name  string
	State string
}

type WorkflowOpts struct {
	Enabled  bool
	Disabled bool
	Id       bool
	Name     bool
}

type workFlowQuery struct {
	Workflows []workflow
}

func ActionWorkflows(cmd *cobra.Command, opts WorkflowOpts) carapace.Action {
	return carapace.ActionCallback(func(c carapace.Context) carapace.Action {
		repo, err := repoOverride(cmd)
		if err != nil {
			return carapace.ActionMessage(err.Error())
		}

		var queryResult workFlowQuery
		return ApiV3Action(cmd, fmt.Sprintf(`repos/%v/%v/actions/workflows`, repo.RepoOwner(), repo.RepoName()), &queryResult, func() carapace.Action {
			vals := make([]string, 0)
			for _, workflow := range queryResult.Workflows {
				if opts.Enabled && workflow.State == "active" ||
					opts.Disabled && workflow.State != "active" {
					if opts.Id {
						vals = append(vals, strconv.Itoa(workflow.Id), workflow.Name)
					}
					if opts.Name {
						vals = append(vals, workflow.Name, strconv.Itoa(workflow.Id))
					}
				}
			}
			return carapace.ActionValuesDescribed(vals...)
		})
	})
}
