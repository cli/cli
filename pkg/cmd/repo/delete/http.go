package delete

import (
	"context"
	"errors"
	"net/http"

	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/githubrest"
	"github.com/cli/cli/v2/internal/safeurl"
)

// deleteRepo stops at a redirect rather than following it, because a repository
// that has been renamed or transferred answers with one, and following it would
// delete whatever now lives at the new location.
func deleteRepo(ctx context.Context, client *githubrest.Client, repo ghrepo.Interface) error {
	url, err := safeurl.JoinPathWithHostPrefix(ghinstance.RESTPrefix(repo.RepoHost()), "repos", repo.RepoOwner(), repo.RepoName())
	if err != nil {
		return err
	}

	req, err := client.NewRequest(ctx, http.MethodDelete, url.String(), nil)
	if err != nil {
		return err
	}

	resp, err := client.SendIgnoringRedirects(req)
	if err != nil {
		var errResp *githubrest.ErrorResponse
		if errors.As(err, &errResp) {
			// The endpoint does not report the scope it needs, so a 403 would
			// otherwise say only that access was denied.
			errResp.RequireScopes("delete_repo")
		}
		if resp != nil {
			resp.Body.Close()
		}
		return err
	}
	defer resp.Body.Close()

	return nil
}
