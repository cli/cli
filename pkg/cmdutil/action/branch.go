package action

import (
	"github.com/rsteube/carapace"
	"github.com/spf13/cobra"
)

type branch struct {
	Name   string
	Target struct{ AbbreviatedOid string } // TODO last commit message?
}

type branchesQuery struct {
	Data struct {
		Repository struct {
			Refs struct {
				Nodes []branch
			}
		}
	}
}

func ActionBranches(cmd *cobra.Command) carapace.Action {
	return carapace.ActionCallback(func(c carapace.Context) carapace.Action {
		var queryResult branchesQuery
		return GraphQlAction(cmd, `repository(owner: $owner, name: $repo){ refs(first: 100, refPrefix: "refs/heads/") { nodes { name, target { abbreviatedOid } } } }`, &queryResult, func() carapace.Action {
			branches := queryResult.Data.Repository.Refs.Nodes
			vals := make([]string, len(branches)*2)
			for index, branch := range branches {
				vals[index*2] = branch.Name
				vals[index*2+1] = branch.Target.AbbreviatedOid
			}
			return carapace.ActionValuesDescribed(vals...)
		})
	})
}
