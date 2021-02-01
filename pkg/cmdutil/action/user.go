package action

import (
	"fmt"
	"strings"

	"github.com/rsteube/carapace"
	"github.com/spf13/cobra"
)

type mentionableUsersQuery struct {
	Data struct {
		Repository struct {
			MentionableUsers struct {
				Nodes []user
			}
		}
	}
}

func ActionMentionableUsers(cmd *cobra.Command) carapace.Action {
	return carapace.ActionCallback(func(c carapace.Context) carapace.Action {
		var queryResult mentionableUsersQuery
		return GraphQlAction(cmd, fmt.Sprintf(`repository(owner: $owner, name: $repo){ mentionableUsers(first: 100, query: "%v") { nodes { login, name } } }`, c.CallbackValue), &queryResult, func() carapace.Action {
			users := queryResult.Data.Repository.MentionableUsers.Nodes
			vals := make([]string, len(users)*2)
			for index, user := range users {
				vals[index*2] = user.Login
				vals[index*2+1] = user.Name
			}
			return carapace.ActionValuesDescribed(vals...)
		})
	})
}

type user struct {
	Login string
	Name  string
}

type assignableUserQuery struct {
	Data struct {
		Repository struct {
			AssignableUsers struct {
				Nodes []user
			}
		}
	}
}

func ActionAssignableUsers(cmd *cobra.Command) carapace.Action {
	return carapace.ActionCallback(func(c carapace.Context) carapace.Action {
		var queryResult assignableUserQuery
		return GraphQlAction(cmd, fmt.Sprintf(`repository(owner: $owner, name: $repo){ assignableUsers(first: 100, query: "%v") { nodes { login, name } } }`, c.CallbackValue), &queryResult, func() carapace.Action {
			users := queryResult.Data.Repository.AssignableUsers.Nodes
			vals := make([]string, len(users)*2)
			for index, user := range users {
				vals[index*2] = user.Login
				vals[index*2+1] = user.Name
			}
			return carapace.ActionValuesDescribed(vals...)
		})
	})
}

type userQuery struct {
	Data struct {
		Search struct {
			Edges []struct {
				Node user
			}
		}
	}
}

type UserOpts struct {
	Users         bool
	Organizations bool
}

func (u UserOpts) format() string {
	filter := make([]string, 0)
	if u.Users {
		filter = append(filter, "... on User { login name }")
	}
	if u.Organizations {
		filter = append(filter, "... on Organization { login name }")
	}
	return strings.Join(filter, " ")
}

func ActionUsers(cmd *cobra.Command, opts UserOpts) carapace.Action {
	return carapace.ActionCallback(func(c carapace.Context) carapace.Action {
		if len(c.CallbackValue) < 2 {
			return carapace.ActionMessage("user search needs at least two characters")
		}

		var queryResult userQuery
		return GraphQlAction(cmd, fmt.Sprintf(`search(query: "%v in:login", type: USER, first: 100) { edges { node { %v } } }`, c.CallbackValue, opts.format()), &queryResult, func() carapace.Action {
			users := queryResult.Data.Search.Edges
			vals := make([]string, len(users)*2)
			for index, user := range users {
				vals[index*2] = user.Node.Login
				vals[index*2+1] = user.Node.Name
			}
			return carapace.ActionValuesDescribed(vals...)
		})
	})

}
