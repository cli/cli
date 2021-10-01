package shared

import (
	"fmt"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
)

func FetchRepository(apiClient *api.Client, repo ghrepo.Interface, fields []string) (*api.Repository, error) {
	query := fmt.Sprintf(`query RepositoryInfo($owner: String!, $name: String!) {
		repository(owner: $owner, name: $name) {%s}
	}`, api.RepositoryGraphQL(fields))

	variables := map[string]interface{}{
		"owner": repo.RepoOwner(),
		"name":  repo.RepoName(),
	}

	var result struct {
		Repository api.Repository
	}
	if err := apiClient.GraphQL(repo.RepoHost(), query, variables, &result); err != nil {
		return nil, err
	}
	return api.InitRepoHostname(&result.Repository, repo.RepoHost()), nil
}
