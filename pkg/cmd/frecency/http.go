package frecency

import (
	"fmt"
	"net/http"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/shurcooL/githubv4"
)

// get most recent PRs
// returns api.Issue for easier database insertion
func getPullRequests(c *http.Client, repo ghrepo.Interface) ([]api.Issue, error) {
	apiClient := api.NewClientFromHTTP(c)
	query := `query GetPRs($owner: String!, $repo: String!) {
  repository(owner: $owner, name: $repo) {
    pullRequests(first: 100, states: [OPEN], orderBy: {field: CREATED_AT, direction: DESC}) {
      nodes {
	  	id
        number
        title
      }
    }
  }
}`
	variables := map[string]interface{}{
		"owner": repo.RepoOwner(),
		"repo":  repo.RepoName(),
	}
	resp := struct {
		Repository struct {
			PullRequests struct {
				Nodes []api.Issue
			}
		}
	}{}

	err := apiClient.GraphQL(repo.RepoHost(), query, variables, &resp)
	if err != nil {
		return nil, err
	}
	return resp.Repository.PullRequests.Nodes, nil
}

// get PRs created after specified time
func getPullRequestsSince(c *http.Client, repo ghrepo.Interface, since time.Time) ([]api.Issue, error) {
	apiClient := api.NewClientFromHTTP(c)
	repoName := ghrepo.FullName(repo)

	// using search since gql `pullRequests` can't be filtered by date
	timeFmt := since.UTC().Format("2006-01-02T15:04:05-0700")
	searchQuery := fmt.Sprintf("repo:%s is:pr is:open created:>%s", repoName, timeFmt)
	query := `query GetPRsSince($query: String!) {
  search(query: $query, type: ISSUE, first: 100) {
    nodes {
      ... on PullRequest {
	  	id
        title
		number
      }
    }
  }
}`
	variables := map[string]interface{}{"query": searchQuery}
	resp := struct {
		Search struct {
			Nodes []api.Issue
		}
	}{}

	err := apiClient.GraphQL(repo.RepoHost(), query, variables, &resp)
	if err != nil {
		return nil, err
	}

	return resp.Search.Nodes, nil
}

// get issues created after specified time
func getIssuesSince(c *http.Client, repo ghrepo.Interface, since time.Time) ([]api.Issue, error) {
	apiClient := api.NewClientFromHTTP(c)
	query := `query GetIssuesSince($owner: String!, $repo: String!, $since: DateTime!, $limit: Int!) {
  repository(owner: $owner, name: $repo) {
    issues(first: $limit, orderBy: {field: CREATED_AT, direction: DESC}, filterBy: {since: $since, states: [OPEN]}) {
      nodes {
	  	id
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
		"limit": 100,
	}
	resp := struct {
		Repository struct {
			Issues struct {
				Nodes []api.Issue
			}
		}
	}{}
	err := apiClient.GraphQL(repo.RepoHost(), query, variables, &resp)
	if err != nil {
		return nil, err
	}

	return resp.Repository.Issues.Nodes, nil
}

// lookup issue or PR by graphQL ID to check if it is open
func isOpen(c *http.Client, repo ghrepo.Interface, args entryWithStats) (bool, error) {
	nodeType := "Issue"
	if args.IsPR {
		nodeType = "PullRequest"
	}
	query := fmt.Sprintf(
		`query GetIssueByID($id: ID!) { node(id: $id) { ... on %s { state } } }`,
		nodeType)

	apiClient := api.NewClientFromHTTP(c)
	variables := map[string]interface{}{"id": githubv4.ID(args.Entry.ID)}
	resp := struct{ Node struct{ State string } }{}
	err := apiClient.GraphQL(repo.RepoHost(), query, variables, &resp)
	if err != nil {
		return false, err
	}
	return resp.Node.State == "OPEN", nil
}
