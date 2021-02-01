package action

import (
	"fmt"

	"github.com/cli/cli/internal/ghrepo"
	"github.com/rsteube/carapace"
	"github.com/rsteube/carapace/pkg/cache"
	"github.com/spf13/cobra"
)

type repository struct {
	Name        string
	Description string
}

type repositoryQuery struct {
	Data struct {
		Search struct {
			Repos []struct {
				Repo repository
			}
		}
	}
}

func repoCacheKey(cmd *cobra.Command) cache.Key {
	return func() (string, error) {
		if repo, err := repoOverride(cmd); err != nil {
			return "", err
		} else {
			return ghrepo.FullName(repo), nil
		}
	}
}

func ActionRepositories(cmd *cobra.Command, owner string, name string) carapace.Action {
	return carapace.ActionCallback(func(c carapace.Context) carapace.Action {
		var queryResult repositoryQuery
		return GraphQlAction(cmd, fmt.Sprintf(`search( type:REPOSITORY, query: """ user:%v "%v" in:name fork:true""", first: 100) { repos: edges { repo: node { ... on Repository { name description } } } }`, owner, name), &queryResult, func() carapace.Action {
			repositories := queryResult.Data.Search.Repos
			vals := make([]string, len(repositories)*2)
			for index, repo := range repositories {
				vals[index*2] = repo.Repo.Name
				vals[index*2+1] = repo.Repo.Description
			}
			return carapace.ActionValuesDescribed(vals...)
		})

	})
}

func ActionOwnerRepositories(cmd *cobra.Command) carapace.Action {
	return carapace.ActionMultiParts("/", func(c carapace.Context) carapace.Action {
		// TODO hack to enable completion outside git repo - this needs to be fixed in GraphQlAction/repooverride though
		if cmd.Flag("repo") == nil {
			cmd.Flags().String("repo", "", "")
		}

		switch len(c.Parts) {
		case 0:
			return ActionUsers(cmd, UserOpts{Users: true, Organizations: true}).Invoke(c).Suffix("/").ToA()
		case 1:
			_ = cmd.Flag("repo").Value.Set(c.Parts[0] + "/" + c.CallbackValue) // TODO part of the repo hack
			return ActionRepositories(cmd, c.Parts[0], c.CallbackValue)
		default:
			return carapace.ActionValues()
		}
	})
}
