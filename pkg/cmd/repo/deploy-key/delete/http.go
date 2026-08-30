package delete

import (
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
)

func deleteDeployKey(httpClient *http.Client, repo ghrepo.Interface, id string) error {
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "keys", id)
	if err != nil {
		return err
	}
	// TODO(api-client-rollout)
	// This line of code is part of a mechanical roll out of the api client.
	// As a follow up, consider whether the api client can be injected to this call site, rather than constructed
	return api.NewClientFromHTTP(httpClient).REST(repo.RepoHost(), "DELETE", path.String(), nil, nil)
}
