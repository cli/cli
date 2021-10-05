package archive

import (
	"context"
	"fmt"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/shurcooL/githubv4"
	"github.com/shurcooL/graphql"
)

func repoArchive(client *api.Client, repo *api.Repository) error {
	var mutation struct {
		ArchiveRepository struct {
			Repository struct {
				ID string
			}
		} `graphql:"archiveRepository(input: $input)"`
	}

	variables := map[string]interface{}{
		"input": githubv4.ArchiveRepositoryInput{
			RepositoryID: repo.ID,
		},
	}

	host := repo.RepoHost()
	gql := graphql.NewClient(ghinstance.GraphQLEndpoint(host), client.HTTP())
	err := gql.MutateNamed(context.Background(), "ArchiveRepository", &mutation, variables)
	return err
}

func fetchRepository(apiClient *api.Client, repo ghrepo.Interface, fields []string) (*api.Repository, error) {
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
