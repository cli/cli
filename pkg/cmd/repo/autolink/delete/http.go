package delete

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/githubrest"
	"github.com/cli/cli/v2/internal/safeurl"
)

type AutolinkDeleter struct {
	GitHubREST func(host string, opts ...githubrest.ClientOption) (*githubrest.Client, error)
}

func (a *AutolinkDeleter) Delete(ctx context.Context, repo ghrepo.Interface, id string) error {
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "autolinks", id)
	if err != nil {
		return err
	}

	client, err := a.GitHubREST(repo.RepoHost())
	if err != nil {
		return err
	}

	req, err := client.NewRequest(ctx, http.MethodDelete, path.String(), nil)
	if err != nil {
		return err
	}
	if _, err := client.Do(req, nil); err != nil {
		var httpErr *githubrest.ErrorResponse
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			return fmt.Errorf("error deleting autolink: HTTP 404: Perhaps you are missing admin rights to the repository? (%s)", httpErr.RequestURL)
		}
		return err
	}

	return nil
}
