package action

import (
	"fmt"
	"strings"

	"github.com/rsteube/carapace"
	"github.com/spf13/cobra"
)

type project struct {
	Name string
	Body string
}

type projectQuery struct {
	Data struct {
		Repository struct {
			Projects struct {
				Nodes []project
			}
		}
	}
}

type ProjectOpts struct {
	Closed bool
	Open   bool
}

func (i *ProjectOpts) states() string {
	states := make([]string, 0)
	for index, include := range []bool{i.Closed, i.Open} {
		if include {
			states = append(states, []string{"CLOSED", "OPEN"}[index])
		}
	}
	return fmt.Sprintf("[%v]", strings.Join(states, ","))
}

func ActionProjects(cmd *cobra.Command, opts ProjectOpts) carapace.Action {
	return carapace.ActionCallback(func(c carapace.Context) carapace.Action {
		var queryResult projectQuery
		return GraphQlAction(cmd, fmt.Sprintf(`repository(owner: $owner, name: $repo){ projects(first: 100, states: %v, orderBy: {direction: DESC, field: UPDATED_AT}) { nodes { name, body } } }`, opts.states()), &queryResult, func() carapace.Action {
			projects := queryResult.Data.Repository.Projects.Nodes
			vals := make([]string, len(projects)*2)
			for index, p := range projects {
				vals[index*2] = p.Name
				vals[index*2+1] = strings.SplitN(p.Body, "\n", 2)[0]
			}
			return carapace.ActionValuesDescribed(vals...)
		})
	})
}
