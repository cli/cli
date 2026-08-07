package delete

import (
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
)

func deleteRepo(httpClient *http.Client, repo ghrepo.Interface) error {
	p, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName())
	if err != nil {
		return err
	}

	resp, err := api.NewClientFromHTTP(httpClient).Request(repo.RepoHost(), http.MethodDelete, p.String(), nil,
		api.WithEndpointScopes("delete_repo"),
		api.WithoutFollowingRedirects(),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}
