package delete

import (
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
)

type AutolinkDeleter struct {
	HTTPClient *http.Client
}

func (a *AutolinkDeleter) Delete(repo ghrepo.Interface, id string) error {
	url, err := safeurl.JoinPathWithHostPrefix(ghinstance.RESTPrefix(repo.RepoHost()), "repos", repo.RepoOwner(), repo.RepoName(), "autolinks", id)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodDelete, url.String(), nil)
	if err != nil {
		return err
	}

	resp, err := a.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("error deleting autolink: HTTP 404: Perhaps you are missing admin rights to the repository? (%s)", url)
	} else if resp.StatusCode > 299 {
		return api.HandleHTTPError(resp)
	}

	return nil
}
