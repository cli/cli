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

	// If the repo is renamed (and we're using the old name), the API will return
	// an HTTP 307 (temporary redirect), which is a METHOD-preserving redirection.
	// That is, upon receiving a 307, the HTTP client will repeat the same HTTP
	// method (in this case, DELETE) but against the new location. We don't want
	// such an implicit behaviour, so we pass the `api.WithoutFollowingRedirects`
	// to make sure an error is returned.
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
