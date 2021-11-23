package frecency

import (
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
)

func getIssues(c *http.Client, repo ghrepo.Interface) ([]api.Issue, error) {
	apiClient := api.NewClientFromHTTP(c)
	query := `query GetIssueNumbers($owner: String!, $repo: String!) {
		repository(owner: $owner, name: $repo) {
			issues(first:100, orderBy: {field: UPDATED_AT, direction: DESC}, states: [OPEN]) {
			    nodes {
					 number
					}
				}
			}
  }`
	variables := map[string]interface{}{
		"owner": repo.RepoOwner(),
		"repo":  repo.RepoName(),
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
