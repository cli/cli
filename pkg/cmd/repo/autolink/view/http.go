package view

import (
	"errors"
	"fmt"
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
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "autolinks", id)
	if err != nil {
		return nil, err
	}

	var autolink shared.Autolink
	// TODO(api-client-rollout)
	// This line of code is part of a mechanical roll out of the api client.
	// As a follow up, consider whether the api client can be injected to this call site, rather than constructed
	err = api.NewClientFromHTTP(a.HTTPClient).REST(repo.RepoHost(), http.MethodGet, path.String(), nil, &autolink)
	if err != nil {
		var httpErr api.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("HTTP 404: Perhaps you are missing admin rights to the repository? (%s)", httpErr.RequestURL)
		}
		return nil, err
	}

	return &autolink, nil
}
