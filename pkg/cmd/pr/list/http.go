package list

import (
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	prShared "github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/githubsearch"
)

func listPullRequests(httpClient *http.Client, repo ghrepo.Interface, filters prShared.FilterOptions, limit int) (*api.PullRequestAndTotalCount, error) {
	if filters.Author != "" || filters.Assignee != "" || filters.Search != "" || len(filters.Labels) > 0 {
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
			$state: [PullRequestState!] = OPEN
		) {
			repository(owner: $owner, name: $repo) {
				pullRequests(
					states: $state,
					baseRefName: $baseBranch,
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

	q := githubsearch.NewQuery()
	q.SetType(githubsearch.PullRequest)
	q.InRepository(ghrepo.FullName(repo))
	q.AddQuery(filters.Search)

	switch filters.State {
	case "open":
		q.SetState(githubsearch.Open)
	case "closed":
		q.SetState(githubsearch.Closed)
	case "merged":
		q.SetState(githubsearch.Merged)
	}

	if filters.Author != "" {
		q.AuthoredBy(filters.Author)
	}
	if filters.Assignee != "" {
		q.AssignedTo(filters.Assignee)
	}
	for _, label := range filters.Labels {
		q.AddLabel(label)
	}
	if filters.BaseBranch != "" {
		q.SetBaseBranch(filters.BaseBranch)
	}

	pageLimit := min(limit, 100)
	variables := map[string]interface{}{
		"q": q.String(),
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
