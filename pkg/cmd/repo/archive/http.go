package archive

import (
	"context"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
	graphql "github.com/cli/shurcooL-graphql"
	"github.com/shurcooL/githubv4"
)

func archiveRepo(client *http.Client, repo *api.Repository) error {
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
	gql := graphql.NewClient(ghinstance.GraphQLEndpoint(host), client)
	err := gql.MutateNamed(context.Background(), "ArchiveRepository", &mutation, variables)
	return err
}
