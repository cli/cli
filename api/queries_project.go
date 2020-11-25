package api

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/cli/cli/internal/ghinstance"
	"github.com/cli/cli/internal/ghrepo"
)

func (c Client) projectREST(url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

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

func (c Client) GetProject(baseRepo ghrepo.Interface, projectID int) ([]byte, error) {
	url := fmt.Sprintf("%sprojects/%d", ghinstance.RESTPrefix(baseRepo.RepoHost()), projectID)

	return c.projectREST(url)
}

func (c Client) GetProjectColumns(baseRepo ghrepo.Interface, projectID int) ([]byte, error) {
	url := fmt.Sprintf("%sprojects/%d/columns",
		ghinstance.RESTPrefix(baseRepo.RepoHost()), projectID)

	return c.projectREST(url)

}

func (c Client) GetProjectCards(baseRepo ghrepo.Interface, columnID int) ([]byte, error) {
	url := fmt.Sprintf("%sprojects/columns/%d/cards",
		ghinstance.RESTPrefix(baseRepo.RepoHost()), columnID)

	return c.projectREST(url)
}

func (c Client) GetProjectCardContent(contentURL string) ([]byte, error) {
	return c.projectREST(contentURL)
}
