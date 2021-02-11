package list

import (
	"fmt"
	"strings"

	"github.com/cli/cli/api"
	"github.com/shurcooL/githubv4"
)

type RepositoryList struct {
	Repositories []Repository
	TotalCount   int
}

func listRepos(client *api.Client, hostname string, limit int, owner string, filter FilterOptions) (*RepositoryList, error) {
	type reposBlock struct {
		TotalCount      int
		RepositoryCount int
		Nodes           []Repository
		PageInfo        struct {
			HasNextPage bool
			EndCursor   string
		}
	}

	type response struct {
		RepositoryOwner struct {
			Repositories reposBlock
		}
		Search reposBlock
	}

	fragment := `
	fragment repo on Repository {
		nameWithOwner
		description
		isFork
		isPrivate
		isArchived
		updatedAt
	}`

	// If `--archived` wasn't specified, use `repositoryOwner.repositores`
	query := fragment + `
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
						...repo
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
		"per_page":  githubv4.Int(perPage),
		"endCursor": (*githubv4.String)(nil),
	}

	hasArchivedFilter := filter.Archived

	if hasArchivedFilter {
		// If `--archived` was specified, use the `search` API rather than
		// `repositoryOwner.repositories`
		query = fragment + `
		query RepoList($per_page: Int!, $endCursor: String, $query: String!) {
			search(first: $per_page, after:$endCursor, type: REPOSITORY, query: $query) {
				repositoryCount
				nodes {
					... on Repository {
						...repo
					}
				}
				pageInfo {
					hasNextPage
					endCursor
				}
			}
		}`

		search := []string{fmt.Sprintf("user:%s archived:true fork:true sort:updated-desc", owner)}

		switch filter.Visibility {
		case "private":
			search = append(search, "is:private")
		case "public":
			search = append(search, "is:public")
		default:
			search = append(search, "is:all")
		}

		variables["query"] = strings.Join(search, " ")
	} else {
		variables["owner"] = githubv4.String(owner)

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

		if hasArchivedFilter {
			repos = append(repos, result.Search.Nodes...)
		} else {
			repos = append(repos, result.RepositoryOwner.Repositories.Nodes...)
		}

		if len(repos) >= limit {
			if len(repos) > limit {
				repos = repos[:limit]
			}
			break pagination
		}

		if !result.RepositoryOwner.Repositories.PageInfo.HasNextPage {
			if !result.Search.PageInfo.HasNextPage {
				break
			}
		}

		variables["endCursor"] = githubv4.String(result.RepositoryOwner.Repositories.PageInfo.EndCursor)
		if hasArchivedFilter {
			variables["endCursor"] = githubv4.String(result.Search.PageInfo.EndCursor)
		}
	}

	totalCount = result.RepositoryOwner.Repositories.TotalCount
	if hasArchivedFilter {
		totalCount = result.Search.RepositoryCount
	}

	listResult := &RepositoryList{
		Repositories: repos,
		TotalCount:   totalCount,
	}

	return listResult, nil
}
