package api

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/cli/cli/internal/ghinstance"
	"github.com/cli/cli/internal/ghrepo"
)

func (c Client) GetProject(baseRepo ghrepo.Interface, projectID string) ([]byte, error) {
	url := fmt.Sprintf("%sprojects/%s",
		ghinstance.RESTPrefix(baseRepo.RepoHost()), projectID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// TODO api() { gh api -H 'accept: application/vnd.github.inertia-preview+json' "$@"; }
	req.Header.Set("Accept", "application/vnd.github.inertia-preview+json; charset=utf-8")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == 404 {
		return nil, &NotFoundError{errors.New("pull request not found")}
	} else if resp.StatusCode != 200 {
		return nil, HandleHTTPError(resp)
	}

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return data, nil
}
