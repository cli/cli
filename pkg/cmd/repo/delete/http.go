package delete

import (
	"context"
	"github.com/cli/cli/v2/internal/githubrest"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
)

func deleteRepo(client *http.Client, repo ghrepo.Interface) error {
	restClient, err := api.NewRESTClient(client, repo.RepoHost(),
		githubrest.WithCheckRedirect(func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}))
	if err != nil {
		return err
	}

	url, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName())
	if err != nil {
		return err
	}

	request, err := restClient.NewRequest(context.Background(), http.MethodDelete, url.String(), nil)
	if err != nil {
		return err
	}

	if _, err := restClient.Do(request, nil); err != nil {
		return api.ErrorNeedsScopes(err, "delete_repo")
	}

	return nil
}
