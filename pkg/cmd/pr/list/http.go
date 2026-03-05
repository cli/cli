package list

import (
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/api"
	fd "github.com/cli/cli/v2/internal/featuredetection"
	"github.com/cli/cli/v2/internal/ghrepo"
	prShared "github.com/cli/cli/v2/pkg/cmd/pr/shared"
)

func shouldUseSearch(filters prShared.FilterOptions) bool {
	return filters.Draft != nil || filters.Author != "" || filters.Assignee != "" || filters.Search != "" || len(filters.Labels) > 0
}

func listPullRequests(httpClient *http.Client, detector fd.Detector, repo ghrepo.Interface, filters prShared.FilterOptions, limit int) (*api.PullRequestAndTotalCount, error) {
	if shouldUseSearch(filters) {
		return searchPullRequests(httpClient, detector, repo, filters, limit)
	}

	return prShared.NewLister(httpClient).List(prShared.ListOptions{
		BaseRepo:     repo,
		LimitResults: limit,
		State:        filters.State,
		BaseBranch:   filters.BaseBranch,
		HeadBranch:   filters.HeadBranch,
		Fields:       filters.Fields,
	})
}

func searchPullRequests(httpClient *http.Client, detector fd.Detector, repo ghrepo.Interface, filters prShared.FilterOptions, limit int) (*api.PullRequestAndTotalCount, error) {
	// TODO advancedIssueSearchCleanup
	// We won't need feature detection when GHES 3.17 support ends, since
	// the advanced issue search is the only available search backend for
	// issues.
	features, err := detector.SearchFeatures()
	if err != nil {
		return nil, err
	}

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
			$type: SearchType!,
			$limit: Int!,
			$endCursor: String,
		) {
			search(query: $q, type: $type, first: $limit, after: $endCursor) {
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

	variables := map[string]interface{}{}

	filters.Repo = ghrepo.FullName(repo)
	filters.Entity = "pr"

	// TODO advancedIssueSearchCleanup
	if features.AdvancedIssueSearchAPI {
		variables["q"] = prShared.SearchQueryBuild(filters, true)
		// TODO advancedIssueSearchCleanup
		if features.AdvancedIssueSearchAPIOptIn {
			variables["type"] = "ISSUE_ADVANCED"
		} else {
			variables["type"] = "ISSUE"
		}
	} else {
		variables["q"] = prShared.SearchQueryBuild(filters, false)
		variables["type"] = "ISSUE"
	}

	pageLimit := min(limit, 100)

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
