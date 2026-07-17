package view

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/repo/autolink/shared"
)

type AutolinkViewer struct {
	HTTPClient *http.Client
}

func (a *AutolinkViewer) View(repo ghrepo.Interface, id string) (*shared.Autolink, error) {
	path := fmt.Sprintf("repos/%s/%s/autolinks/%s", repo.RepoOwner(), repo.RepoName(), id)
	var autolink shared.Autolink
	err := api.NewClientFromHTTP(a.HTTPClient).REST(repo.RepoHost(), http.MethodGet, path, nil, &autolink)
	if err != nil {
		var httpErr api.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("HTTP 404: Perhaps you are missing admin rights to the repository? (https://api.github.com/%s)", path)
		}
		return nil, err
	}
	return &autolink, nil
}
