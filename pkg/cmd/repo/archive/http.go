package archive

import (
	"net/http"

	"github.com/cli/cli/v2/api"
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

	gql := api.NewClientFromHTTP(client)
	err := gql.Mutate(repo.RepoHost(), "ArchiveRepository", &mutation, variables)
	return err
}
