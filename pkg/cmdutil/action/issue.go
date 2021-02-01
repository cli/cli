package action

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/rsteube/carapace"
	"github.com/spf13/cobra"
)

type issue struct {
	Number int
	Title  string
}

type issueQuery struct {
	Data struct {
		Repository struct {
			Issues struct {
				Nodes []issue
			}
		}
	}
}

type IssueOpts struct {
	Closed bool
	Open   bool
}

func (i *IssueOpts) states() string {
	states := make([]string, 0)
	for index, include := range []bool{i.Closed, i.Open} {
		if include {
			states = append(states, []string{"CLOSED", "OPEN"}[index])
		}
	}
	return fmt.Sprintf("[%v]", strings.Join(states, ","))
}

func ActionIssues(cmd *cobra.Command, opts IssueOpts) carapace.Action {
	return carapace.ActionCallback(func(c carapace.Context) carapace.Action {
		var queryResult issueQuery
		return GraphQlAction(cmd, fmt.Sprintf(`repository(owner: $owner, name: $repo){ issues(first: 100, states: %v, orderBy: {direction: DESC, field: UPDATED_AT}) { nodes { number, title } } }`, opts.states()), &queryResult, func() carapace.Action {
			issues := queryResult.Data.Repository.Issues.Nodes
			vals := make([]string, len(issues)*2)
			for index, issue := range issues {
				vals[index*2] = strconv.Itoa(issue.Number)
				vals[index*2+1] = issue.Title // TODO shorten title
			}
			return carapace.ActionValuesDescribed(vals...)
		})
	})
}
