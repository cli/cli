package list

import (
	"strings"

	"github.com/cli/cli/api"
	"github.com/shurcooL/githubv4"
)

type RepositoryList struct {
	Repositories []Repository
	TotalCount   int
}

func listRepos(client *api.Client, hostname string, limit int, owner string, filter FilterOptions) (*RepositoryList, error) {
	type response struct {
		RepositoryOwner struct {
			Repositories struct {
				TotalCount      int
				RepositoryCount int
				Nodes           []Repository
				PageInfo        struct {
					HasNextPage bool
					EndCursor   string
				}
			}
		}
	}

	query := `
	query RepoList($owner: String!, $per_page: Int!, $endCursor: String, $fork: Boolean, $privacy: RepositoryPrivacy) {
		repositoryOwner(login: $owner) {
			repositories(
				first: $per_page,
				after: $endCursor,
				privacy: $privacy,
				isFork: $fork,
				ownerAffiliations: OWNER,
				orderBy: { field: UPDATED_AT, direction: DESC }) {
					totalCount
					nodes {
						nameWithOwner
						description
						isFork
						isPrivate
						isArchived
						updatedAt
					}
					pageInfo {
						hasNextPage
						endCursor
					}
			}
		}
	}`

	perPage := limit
	if perPage > 100 {
		perPage = 100
	}

	variables := map[string]interface{}{
		"owner":     githubv4.String(owner),
		"per_page":  githubv4.Int(perPage),
		"endCursor": (*githubv4.String)(nil),
	}

	if filter.Visibility != "" {
		variables["privacy"] = githubv4.RepositoryPrivacy(strings.ToUpper(filter.Visibility))
	} else {
		variables["privacy"] = (*githubv4.RepositoryPrivacy)(nil)
	}

	if filter.Fork {
		variables["fork"] = githubv4.Boolean(true)
	} else if filter.Source {
		variables["fork"] = githubv4.Boolean(false)
	} else {
		variables["fork"] = (*githubv4.Boolean)(nil)
	}

	var repos []Repository
	var totalCount int

	var result response

pagination:
	for {
		err := client.GraphQL(hostname, query, variables, &result)
		if err != nil {
			return nil, err
		}

		repos = append(repos, result.RepositoryOwner.Repositories.Nodes...)

		if len(repos) >= limit {
			if len(repos) > limit {
				repos = repos[:limit]
			}
			break pagination
		}

		if !result.RepositoryOwner.Repositories.PageInfo.HasNextPage {
			break
		}

		variables["endCursor"] = githubv4.String(result.RepositoryOwner.Repositories.PageInfo.EndCursor)
	}

	totalCount = result.RepositoryOwner.Repositories.TotalCount

	listResult := &RepositoryList{
		Repositories: repos,
		TotalCount:   totalCount,
	}

	return listResult, nil
}
