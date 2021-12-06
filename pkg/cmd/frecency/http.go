package frecency

import (
	"fmt"
	"net/http"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
)

func getIssues(c *http.Client, repo ghrepo.Interface, since time.Time) ([]api.Issue, error) {
	apiClient := api.NewClientFromHTTP(c)
	query := `
query GetIssues($owner: String!, $repo: String!, $since: DateTime!) {
  repository(owner: $owner, name: $repo) {
    issues(first: 100, orderBy: {field: CREATED_AT, direction: DESC}, filterBy: {since: $since, states: [OPEN]}) {
      nodes {
        number
        title
      }
    }
  }
}`
	variables := map[string]interface{}{
		"owner": repo.RepoOwner(),
		"repo":  repo.RepoName(),
		"since": since.UTC().Format("2006-01-02T15:04:05-0700"),
	}
	type responseData struct {
		Repository struct {
			Issues struct {
				Nodes []api.Issue
			}
		}
	}
	var resp responseData
	err := apiClient.GraphQL(repo.RepoHost(), query, variables, &resp)
	if err != nil {
		return nil, err
	}

	return resp.Repository.Issues.Nodes, nil
}

func getPullRequests(c *http.Client, repo ghrepo.Interface, since time.Time) ([]api.Issue, error) {
	apiClient := api.NewClientFromHTTP(c)
	repoName := ghrepo.FullName(repo)

	// using search since gql `pullRequests` can't be filtered by date
	timeFmt := since.UTC().Format("2006-01-02T15:04:05-0700")
	searchQuery := fmt.Sprintf("repo:%s is:pr is:open created:>%s", repoName, timeFmt)
	query := `
query getPRs($query: String!, $limit: Int!) {
  search(query: $query, type: ISSUE, first: $limit) {
    nodes {
      ... on PullRequest {
        title
		number
      }
    }
  }
}`
	variables := map[string]interface{}{
		"query": searchQuery,
		"limit": 100,
	}
	type responseData struct{ Search struct{ Nodes []api.Issue } }

	var resp responseData
	err := apiClient.GraphQL(repo.RepoHost(), query, variables, &resp)
	if err != nil {
		return nil, err
	}
	return resp.Search.Nodes, nil
}
