package api

import (
	"context"
	"time"

	"github.com/shurcooL/githubv4"
)

type SearchRepoResult struct {
	NameWithOwner string
	Description   string
	Stargazers    struct {
		TotalCount int
	}
}

func SearchRepos(client *Client, q string, limit int) ([]SearchRepoResult, error) {
	var query struct {
		Search struct {
			Nodes []struct {
				Repository SearchRepoResult `graphql:"...on Repository"`
			}
			PageInfo struct {
				HasNextPage bool
				EndCursor   string
			}
		} `graphql:"search(query: $query, type: REPOSITORY, first: $limit, after: $endCursor)"`
	}

	perPage := limit
	if perPage > 100 {
		perPage = 100
	}
	variables := map[string]interface{}{
		"query":     githubv4.String(q),
		"limit":     githubv4.Int(perPage),
		"endCursor": (*githubv4.String)(nil),
	}

	v4 := githubv4.NewClient(client.http)

	var results []SearchRepoResult

pagination:
	for {
		err := v4.Query(context.Background(), &query, variables)
		if err != nil {
			return nil, err
		}
		for _, n := range query.Search.Nodes {
			results = append(results, n.Repository)
			if len(results) == limit {
				break pagination
			}
		}
		if !query.Search.PageInfo.HasNextPage {
			break
		}
		variables["endCursor"] = githubv4.String(query.Search.PageInfo.EndCursor)
	}

	return results, nil
}

type SearchIssueResult struct {
	Title      string
	Number     int
	State      string
	CreatedAt  time.Time
	Repository struct {
		NameWithOwner string
	}
}

func SearchIssues(client *Client, q string, limit int) ([]SearchIssueResult, error) {
	var query struct {
		Search struct {
			Nodes []struct {
				Issue       SearchIssueResult `graphql:"...on Issue"`
				PullRequest SearchIssueResult `graphql:"...on PullRequest"`
			}
			PageInfo struct {
				HasNextPage bool
				EndCursor   string
			}
		} `graphql:"search(query: $query, type: ISSUE, first: $limit, after: $endCursor)"`
	}

	perPage := limit
	if perPage > 100 {
		perPage = 100
	}
	variables := map[string]interface{}{
		"query":     githubv4.String(q),
		"limit":     githubv4.Int(perPage),
		"endCursor": (*githubv4.String)(nil),
	}

	v4 := githubv4.NewClient(client.http)

	var results []SearchIssueResult

pagination:
	for {
		err := v4.Query(context.Background(), &query, variables)
		if err != nil {
			return nil, err
		}
		for _, n := range query.Search.Nodes {
			if n.Issue.Number > 0 {
				results = append(results, n.Issue)
			} else {
				results = append(results, n.PullRequest)
			}
			if len(results) == limit {
				break pagination
			}
		}
		if !query.Search.PageInfo.HasNextPage {
			break
		}
		variables["endCursor"] = githubv4.String(query.Search.PageInfo.EndCursor)
	}

	return results, nil
}
