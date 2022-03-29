package list

import (
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	prShared "github.com/cli/cli/v2/pkg/cmd/pr/shared"
)

func shouldUseSearch(filters prShared.FilterOptions) bool {
	return filters.Draft != nil || filters.Author != "" || filters.Assignee != "" || filters.Search != "" || len(filters.Labels) > 0
}

func listPullRequests(httpClient *http.Client, repo ghrepo.Interface, filters prShared.FilterOptions, limit int) (*api.PullRequestAndTotalCount, error) {
	if shouldUseSearch(filters) {
		return searchPullRequests(httpClient, repo, filters, limit)
	}

	type response struct {
		Repository struct {
			PullRequests struct {
				Nodes    []api.PullRequest
				PageInfo struct {
					HasNextPage bool
					EndCursor   string
				}
				TotalCount int
			}
		}
	}

	fragment := fmt.Sprintf("fragment pr on PullRequest{%s}", api.PullRequestGraphQL(filters.Fields))
	query := fragment + `
		query PullRequestList(
			$owner: String!,
			$repo: String!,
			$limit: Int!,
			$endCursor: String,
			$baseBranch: String,
			$headBranch: String,
			$state: [PullRequestState!] = OPEN
		) {
			repository(owner: $owner, name: $repo) {
				pullRequests(
					states: $state,
					baseRefName: $baseBranch,
					headRefName: $headBranch,
					first: $limit,
					after: $endCursor,
					orderBy: {field: CREATED_AT, direction: DESC}
				) {
					totalCount
					nodes {
						...pr
					}
					pageInfo {
						hasNextPage
						endCursor
					}
				}
			}
		}`

	pageLimit := min(limit, 100)
	variables := map[string]interface{}{
		"owner": repo.RepoOwner(),
		"repo":  repo.RepoName(),
	}

	switch filters.State {
	case "open":
		variables["state"] = []string{"OPEN"}
	case "closed":
		variables["state"] = []string{"CLOSED", "MERGED"}
	case "merged":
		variables["state"] = []string{"MERGED"}
	case "all":
		variables["state"] = []string{"OPEN", "CLOSED", "MERGED"}
	default:
		return nil, fmt.Errorf("invalid state: %s", filters.State)
	}

	if filters.BaseBranch != "" {
		variables["baseBranch"] = filters.BaseBranch
	}
	if filters.HeadBranch != "" {
		variables["headBranch"] = filters.HeadBranch
	}

	res := api.PullRequestAndTotalCount{}
	var check = make(map[int]struct{})
	client := api.NewClientFromHTTP(httpClient)

loop:
	for {
		variables["limit"] = pageLimit
		var data response
		err := client.GraphQL(repo.RepoHost(), query, variables, &data)
		if err != nil {
			return nil, err
		}
		prData := data.Repository.PullRequests
		res.TotalCount = prData.TotalCount

		for _, pr := range prData.Nodes {
			if _, exists := check[pr.Number]; exists && pr.Number > 0 {
				continue
			}
			check[pr.Number] = struct{}{}

			res.PullRequests = append(res.PullRequests, pr)
			if len(res.PullRequests) == limit {
				break loop
			}
		}

		if prData.PageInfo.HasNextPage {
			variables["endCursor"] = prData.PageInfo.EndCursor
			pageLimit = min(pageLimit, limit-len(res.PullRequests))
		} else {
			break
		}
	}

	return &res, nil
}

func searchPullRequests(httpClient *http.Client, repo ghrepo.Interface, filters prShared.FilterOptions, limit int) (*api.PullRequestAndTotalCount, error) {
	type response struct {
		Search struct {
			Nodes    []api.PullRequest
			PageInfo struct {
				HasNextPage bool
				EndCursor   string
			}
			IssueCount int
		}
	}

	fragment := fmt.Sprintf("fragment pr on PullRequest{%s}", api.PullRequestGraphQL(filters.Fields))
	query := fragment + `
		query PullRequestSearch(
			$q: String!,
			$limit: Int!,
			$endCursor: String,
		) {
			search(query: $q, type: ISSUE, first: $limit, after: $endCursor) {
				issueCount
				nodes {
					...pr
				}
				pageInfo {
					hasNextPage
					endCursor
				}
			}
		}`

	filters.Repo = ghrepo.FullName(repo)
	filters.Entity = "pr"
	q := prShared.SearchQueryBuild(filters)

	pageLimit := min(limit, 100)
	variables := map[string]interface{}{"q": q}

	res := api.PullRequestAndTotalCount{SearchCapped: limit > 1000}
	var check = make(map[int]struct{})
	client := api.NewClientFromHTTP(httpClient)

loop:
	for {
		variables["limit"] = pageLimit
		var data response
		err := client.GraphQL(repo.RepoHost(), query, variables, &data)
		if err != nil {
			return nil, err
		}
		prData := data.Search
		res.TotalCount = prData.IssueCount

		for _, pr := range prData.Nodes {
			if _, exists := check[pr.Number]; exists && pr.Number > 0 {
				continue
			}
			check[pr.Number] = struct{}{}

			res.PullRequests = append(res.PullRequests, pr)
			if len(res.PullRequests) == limit {
				break loop
			}
		}

		if prData.PageInfo.HasNextPage {
			variables["endCursor"] = prData.PageInfo.EndCursor
			pageLimit = min(pageLimit, limit-len(res.PullRequests))
		} else {
			break
		}
	}

	return &res, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
