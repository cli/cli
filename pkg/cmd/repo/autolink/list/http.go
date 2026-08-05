package list

import (
	"context"
	"errors"
	"fmt"
	"github.com/cli/cli/v2/internal/githubrest"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
	"github.com/cli/cli/v2/pkg/cmd/repo/autolink/shared"
)

type AutolinkLister struct {
	HTTPClient *http.Client
}

func (a *AutolinkLister) List(repo ghrepo.Interface) ([]shared.Autolink, error) {
	client, err := api.NewRESTClient(a.HTTPClient, repo.RepoHost())
	if err != nil {
		return nil, err
	}

	url, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "autolinks")
	if err != nil {
		return nil, err
	}

	req, err := client.NewRequest(context.Background(), http.MethodGet, url.String(), nil)
	if err != nil {
		return nil, err
	}

	var autolinks []shared.Autolink
	if _, err := client.Do(req, &autolinks); err != nil {
		var httpErr *githubrest.ErrorResponse
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("error getting autolinks: HTTP 404: Perhaps you are missing admin rights to the repository? (%s)", httpErr.RequestURL)
		}
		return nil, err
	}

	return autolinks, nil
}
