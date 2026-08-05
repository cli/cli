package view

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

type AutolinkViewer struct {
	HTTPClient *http.Client
}

func (a *AutolinkViewer) View(repo ghrepo.Interface, id string) (*shared.Autolink, error) {
	client, err := api.NewRESTClient(a.HTTPClient, repo.RepoHost())
	if err != nil {
		return nil, err
	}

	url, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "autolinks", id)
	if err != nil {
		return nil, err
	}

	req, err := client.NewRequest(context.Background(), http.MethodGet, url.String(), nil)
	if err != nil {
		return nil, err
	}

	var autolink shared.Autolink
	if _, err := client.Do(req, &autolink); err != nil {
		var httpErr *githubrest.ErrorResponse
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("HTTP 404: Perhaps you are missing admin rights to the repository? (%s)", httpErr.RequestURL)
		}
		return nil, err
	}

	return &autolink, nil
}
