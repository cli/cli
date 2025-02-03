package view

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/repo/autolink/shared"
)

type AutolinkViewer struct {
	HTTPClient *http.Client
}

func (a *AutolinkViewer) View(repo ghrepo.Interface, id string) (*shared.Autolink, error) {
	path := fmt.Sprintf("repos/%s/%s/autolinks/%s", repo.RepoOwner(), repo.RepoName(), id)
	url := ghinstance.RESTPrefix(repo.RepoHost()) + path
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := a.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("HTTP 404: Either no autolink with this ID exists for this repository or you are missing admin rights to the repository. (https://api.github.com/%s)", path)
	} else if resp.StatusCode > 299 {
		return nil, api.HandleHTTPError(resp)
	}

	var autolink shared.Autolink
	err = json.NewDecoder(resp.Body).Decode(&autolink)

	if err != nil {
		return nil, err
	}

	return &autolink, nil
}
