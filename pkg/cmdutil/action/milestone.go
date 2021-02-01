package action

import (
	"github.com/rsteube/carapace"
	"github.com/spf13/cobra"
)

type milestone struct {
	Title       string
	Description string
}

type milestoneQuery struct {
	Data struct {
		Repository struct {
			Milestones struct {
				Nodes []milestone
			}
		}
	}
}

func ActionMilestones(cmd *cobra.Command) carapace.Action {
	return carapace.ActionCallback(func(c carapace.Context) carapace.Action {
		var queryResult milestoneQuery
		return GraphQlAction(cmd, `repository(owner: $owner, name: $repo){ milestones(first: 100) { nodes { title, description } } }`, &queryResult, func() carapace.Action {
			milestones := queryResult.Data.Repository.Milestones.Nodes
			vals := make([]string, len(milestones)*2)
			for index, repo := range milestones {
				vals[index*2] = repo.Title
				vals[index*2+1] = repo.Description
			}
			return carapace.ActionValuesDescribed(vals...)
		})
	})
}
