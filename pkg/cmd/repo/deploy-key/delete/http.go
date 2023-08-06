package delete

import (
	"fmt"
	"io"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
)

func deleteDeployKey(httpClient *http.Client, repo ghrepo.Interface, id string) error {
	path := fmt.Sprintf("repos/%s/%s/keys/%s", repo.RepoOwner(), repo.RepoName(), id)
	url := ghinstance.RESTPrefix(repo.RepoHost()) + path

	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode > 299 {
		return api.HandleHTTPError(resp)
	}

	_, err = io.Copy(io.Discard, resp.Body)
	if err != nil {
		return err
	}

	return nil
}
