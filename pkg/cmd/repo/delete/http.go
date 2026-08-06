package delete

import (
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
)

func deleteRepo(client *http.Client, repo ghrepo.Interface) error {
	oldClient := *client
	client = &oldClient
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName())
	if err != nil {
		return err
	}

	// TODO(api-client-rollout)
	// This line of code is part of a mechanical roll out of the api client.
	// As a follow up, consider whether the api client can be injected to this call site, rather than constructed
	resp, err := api.NewClientFromHTTP(client).Request(repo.RepoHost(), http.MethodDelete, path.String(), nil,
		api.WithEndpointScopes("delete_repo"))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}
