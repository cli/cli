package list

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/githubrest"
	"github.com/cli/cli/v2/internal/safeurl"
	"github.com/cli/cli/v2/pkg/cmd/repo/autolink/shared"
)

type AutolinkLister struct {
	GitHubREST func(host string, opts ...githubrest.ClientOption) (*githubrest.Client, error)
}

func (a *AutolinkLister) List(ctx context.Context, repo ghrepo.Interface) ([]shared.Autolink, error) {
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "autolinks")
	if err != nil {
		return nil, err
	}

	client, err := a.GitHubREST(repo.RepoHost())
	if err != nil {
		return nil, err
	}

	var autolinks []shared.Autolink
	req, err := client.NewRequest(ctx, http.MethodGet, path.String(), nil)
	if err != nil {
		return nil, err
	}
	if _, err := client.Do(req, &autolinks); err != nil {
		var httpErr *githubrest.ErrorResponse
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("error getting autolinks: HTTP 404: Perhaps you are missing admin rights to the repository? (%s)", httpErr.RequestURL)
		}
		return nil, err
	}

	return autolinks, nil
}
