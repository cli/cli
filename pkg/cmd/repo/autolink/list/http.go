package list

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
)

type AutolinkLister struct {
	HTTPClient *http.Client
}

func (a *AutolinkLister) List(repo ghrepo.Interface) ([]autolink, error) {
	path := fmt.Sprintf("repos/%s/%s/autolinks", repo.RepoOwner(), repo.RepoName())
	url := ghinstance.RESTPrefix(repo.RepoHost()) + path
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := a.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("error getting autolinks: HTTP 404: Perhaps you are missing admin rights to the repository? (https://api.github.com/%s)", path)
	} else if resp.StatusCode > 299 {
		return nil, api.HandleHTTPError(resp)
	}
	var autolinks []autolink
	err = json.NewDecoder(resp.Body).Decode(&autolinks)
	if err != nil {
		return nil, err
	}

	return autolinks, nil
}
