package list

import (
	"fmt"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	prShared "github.com/cli/cli/v2/pkg/cmd/pr/shared"
)

func listIssues(client *api.Client, repo ghrepo.Interface, filters prShared.FilterOptions, limit int) (*api.IssuesAndTotalCount, error) {
	var states []string
	switch filters.State {
	case "open", "":
		states = []string{"OPEN"}
	case "closed":
		states = []string{"CLOSED"}
	case "all":
		states = []string{"OPEN", "CLOSED"}
	default:
		return nil, fmt.Errorf("invalid state: %s", filters.State)
	}

	fragments := fmt.Sprintf("fragment issue on Issue {%s}", api.IssueGraphQL(filters.Fields))
	query := fragments + `
	query IssueList($owner: String!, $repo: String!, $limit: Int, $endCursor: String, $states: [IssueState!] = OPEN, $assignee: String, $author: String, $mention: String) {
		repository(owner: $owner, name: $repo) {
			hasIssuesEnabled
			issues(first: $limit, after: $endCursor, orderBy: {field: CREATED_AT, direction: DESC}, states: $states, filterBy: {assignee: $assignee, createdBy: $author, mentioned: $mention}) {
				totalCount
				nodes {
					...issue
				}
				pageInfo {
					hasNextPage
					endCursor
				}
			}
		}
	}
	`

	variables := map[string]interface{}{
		"owner":  repo.RepoOwner(),
		"repo":   repo.RepoName(),
		"states": states,
	}
	if filters.Assignee != "" {
		variables["assignee"] = filters.Assignee
	}
	if filters.Author != "" {
		variables["author"] = filters.Author
	}
	if filters.Mention != "" {
		variables["mention"] = filters.Mention
	}

	if filters.Milestone != "" {
		// The "milestone" filter in the GraphQL connection doesn't work as documented and accepts neither a
		// milestone number nor a title. It does accept a numeric database ID, but we cannot obtain one
		// using the GraphQL API.
		return nil, fmt.Errorf("cannot filter by milestone using the `Repository.issues` GraphQL connection")
	}

	type responseData struct {
		Repository struct {
			Issues struct {
				TotalCount int
				Nodes      []api.Issue
				PageInfo   struct {
					HasNextPage bool
					EndCursor   string
				}
			}
			HasIssuesEnabled bool
		}
	}

	var issues []api.Issue
	var totalCount int
	pageLimit := min(limit, 100)

loop:
	for {
		var response responseData
		variables["limit"] = pageLimit
		err := client.GraphQL(repo.RepoHost(), query, variables, &response)
		if err != nil {
			return nil, err
		}
		if !response.Repository.HasIssuesEnabled {
			return nil, fmt.Errorf("the '%s' repository has disabled issues", ghrepo.FullName(repo))
		}
		totalCount = response.Repository.Issues.TotalCount

		for _, issue := range response.Repository.Issues.Nodes {
			issues = append(issues, issue)
			if len(issues) == limit {
				break loop
			}
		}

		if response.Repository.Issues.PageInfo.HasNextPage {
			variables["endCursor"] = response.Repository.Issues.PageInfo.EndCursor
			pageLimit = min(pageLimit, limit-len(issues))
		} else {
			break
		}
	}

	res := api.IssuesAndTotalCount{Issues: issues, TotalCount: totalCount}
	return &res, nil
}

func searchIssues(client *api.Client, repo ghrepo.Interface, filters prShared.FilterOptions, limit int) (*api.IssuesAndTotalCount, error) {
	fragments := fmt.Sprintf("fragment issue on Issue {%s}", api.IssueGraphQL(filters.Fields))
	query := fragments +
		`query IssueSearch($repo: String!, $owner: String!, $type: SearchType!, $limit: Int, $after: String, $query: String!) {
			repository(name: $repo, owner: $owner) {
				hasIssuesEnabled
			}
			search(type: $type, last: $limit, after: $after, query: $query) {
				issueCount
				nodes { ...issue }
				pageInfo {
					hasNextPage
					endCursor
				}
			}
		}`

	type response struct {
		Repository struct {
			HasIssuesEnabled bool
		}
		Search struct {
			IssueCount int
			Nodes      []api.Issue
			PageInfo   struct {
				HasNextPage bool
				EndCursor   string
			}
		}
	}

	filters.Repo = ghrepo.FullName(repo)
	filters.Entity = "issue"
	q := prShared.SearchQueryBuild(filters)

	perPage := min(limit, 100)

	variables := map[string]interface{}{
		"owner": repo.RepoOwner(),
		"repo":  repo.RepoName(),
		"type":  "ISSUE",
		"limit": perPage,
		"query": q,
	}

	ic := api.IssuesAndTotalCount{SearchCapped: limit > 1000}

loop:
	for {
		var resp response
		err := client.GraphQL(repo.RepoHost(), query, variables, &resp)
		if err != nil {
			return nil, err
		}

		if !resp.Repository.HasIssuesEnabled {
			return nil, fmt.Errorf("the '%s' repository has disabled issues", ghrepo.FullName(repo))
		}

		ic.TotalCount = resp.Search.IssueCount

		for _, issue := range resp.Search.Nodes {
			ic.Issues = append(ic.Issues, issue)
			if len(ic.Issues) == limit {
				break loop
			}
		}

		if !resp.Search.PageInfo.HasNextPage {
			break
		}
		variables["after"] = resp.Search.PageInfo.EndCursor
		variables["perPage"] = min(perPage, limit-len(ic.Issues))
	}

	return &ic, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
