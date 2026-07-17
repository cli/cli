package list

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/repo/autolink/shared"
)

type AutolinkLister struct {
	HTTPClient *http.Client
}

func (a *AutolinkLister) List(repo ghrepo.Interface) ([]shared.Autolink, error) {
	path := fmt.Sprintf("repos/%s/%s/autolinks", repo.RepoOwner(), repo.RepoName())
	var autolinks []shared.Autolink
	err := api.NewClientFromHTTP(a.HTTPClient).REST(repo.RepoHost(), http.MethodGet, path, nil, &autolinks)
	if err != nil {
		var httpErr api.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("error getting autolinks: HTTP 404: Perhaps you are missing admin rights to the repository? (https://api.github.com/%s)", path)
		}
		return nil, err
	}
	return autolinks, nil
}
