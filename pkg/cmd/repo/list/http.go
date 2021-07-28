package list

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/cli/cli/api"
	"github.com/cli/cli/pkg/githubsearch"
	"github.com/shurcooL/githubv4"
)

type RepositoryList struct {
	Owner        string
	Repositories []api.Repository
	TotalCount   int
	FromSearch   bool
}

type FilterOptions struct {
	Visibility  string // private, public
	Fork        bool
	Source      bool
	Language    string
	Archived    bool
	NonArchived bool
	Fields      []string
}

func listRepos(client *http.Client, hostname string, limit int, owner string, filter FilterOptions) (*RepositoryList, error) {
	if filter.Language != "" || filter.Archived || filter.NonArchived {
		return searchRepos(client, hostname, limit, owner, filter)
	}

	perPage := limit
	if perPage > 100 {
		perPage = 100
	}

	variables := map[string]interface{}{
		"perPage": githubv4.Int(perPage),
	}

	if filter.Visibility != "" {
		variables["privacy"] = githubv4.RepositoryPrivacy(strings.ToUpper(filter.Visibility))
	}

	if filter.Fork {
		variables["fork"] = githubv4.Boolean(true)
	} else if filter.Source {
		variables["fork"] = githubv4.Boolean(false)
	}

	inputs := []string{"$perPage:Int!", "$endCursor:String", "$privacy:RepositoryPrivacy", "$fork:Boolean"}
	var ownerConnection string
	if owner == "" {
		ownerConnection = "repositoryOwner: viewer"
	} else {
		ownerConnection = "repositoryOwner(login: $owner)"
		variables["owner"] = githubv4.String(owner)
		inputs = append(inputs, "$owner:String!")
	}

	type result struct {
		RepositoryOwner struct {
			Login        string
			Repositories struct {
				Nodes      []api.Repository
				TotalCount int
				PageInfo   struct {
					HasNextPage bool
					EndCursor   string
				}
			}
		}
	}

	query := fmt.Sprintf(`query RepositoryList(%s) {
		%s {
			login
			repositories(first: $perPage, after: $endCursor, privacy: $privacy, isFork: $fork, ownerAffiliations: OWNER, orderBy: { field: PUSHED_AT, direction: DESC }) {
				nodes{%s}
				totalCount
				pageInfo{hasNextPage,endCursor}
			}
		}
	}`, strings.Join(inputs, ","), ownerConnection, api.RepositoryGraphQL(filter.Fields))

	apiClient := api.NewClientFromHTTP(client)
	listResult := RepositoryList{}
pagination:
	for {
		var res result
		err := apiClient.GraphQL(hostname, query, variables, &res)
		if err != nil {
			return nil, err
		}

		owner := res.RepositoryOwner
		listResult.TotalCount = owner.Repositories.TotalCount
		listResult.Owner = owner.Login

		for _, repo := range owner.Repositories.Nodes {
			listResult.Repositories = append(listResult.Repositories, repo)
			if len(listResult.Repositories) >= limit {
				break pagination
			}
		}

		if !owner.Repositories.PageInfo.HasNextPage {
			break
		}
		variables["endCursor"] = githubv4.String(owner.Repositories.PageInfo.EndCursor)
	}

	return &listResult, nil
}

func searchRepos(client *http.Client, hostname string, limit int, owner string, filter FilterOptions) (*RepositoryList, error) {
	type result struct {
		Search struct {
			RepositoryCount int
			Nodes           []api.Repository
			PageInfo        struct {
				HasNextPage bool
				EndCursor   string
			}
		}
	}

	query := fmt.Sprintf(`query RepositoryListSearch($query:String!,$perPage:Int!,$endCursor:String) {
		search(type: REPOSITORY, query: $query, first: $perPage, after: $endCursor) {
			repositoryCount
			nodes{...on Repository{%s}}
			pageInfo{hasNextPage,endCursor}
		}
	}`, api.RepositoryGraphQL(filter.Fields))

	perPage := limit
	if perPage > 100 {
		perPage = 100
	}

	variables := map[string]interface{}{
		"query":   githubv4.String(searchQuery(owner, filter)),
		"perPage": githubv4.Int(perPage),
	}

	apiClient := api.NewClientFromHTTP(client)
	listResult := RepositoryList{FromSearch: true}
pagination:
	for {
		var result result
		err := apiClient.GraphQL(hostname, query, variables, &result)
		if err != nil {
			return nil, err
		}

		listResult.TotalCount = result.Search.RepositoryCount
		for _, repo := range result.Search.Nodes {
			if listResult.Owner == "" && repo.NameWithOwner != "" {
				idx := strings.IndexRune(repo.NameWithOwner, '/')
				listResult.Owner = repo.NameWithOwner[:idx]
			}
			listResult.Repositories = append(listResult.Repositories, repo)
			if len(listResult.Repositories) >= limit {
				break pagination
			}
		}

		if !result.Search.PageInfo.HasNextPage {
			break
		}
		variables["endCursor"] = githubv4.String(result.Search.PageInfo.EndCursor)
	}

	return &listResult, nil
}

func searchQuery(owner string, filter FilterOptions) string {
	q := githubsearch.NewQuery()
	q.SortBy(githubsearch.UpdatedAt, githubsearch.Desc)

	if owner == "" {
		q.OwnedBy("@me")
	} else {
		q.OwnedBy(owner)
	}

	if filter.Fork {
		q.OnlyForks()
	} else {
		q.IncludeForks(!filter.Source)
	}

	if filter.Language != "" {
		q.SetLanguage(filter.Language)
	}

	switch filter.Visibility {
	case "public":
		q.SetVisibility(githubsearch.Public)
	case "private":
		q.SetVisibility(githubsearch.Private)
	}

	if filter.Archived {
		q.SetArchived(true)
	} else if filter.NonArchived {
		q.SetArchived(false)
	}

	return q.String()
}
