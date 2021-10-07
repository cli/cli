package delete

import (
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
)

func deleteRepo(client *http.Client, repo ghrepo.Interface) error {
	url := fmt.Sprintf("%srepos/%s",
		ghinstance.RESTPrefix(repo.RepoHost()),
		ghrepo.FullName(repo))

	request, err := http.NewRequest("DELETE", url, nil)
	request.Header.Set("Accept", "application/vnd.github.v3+json")
	if err != nil {
		return err
	}

	resp, err := client.Do(request)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	err = api.HandleHTTPError(resp)
	if resp.StatusCode == 403 {
		return fmt.Errorf(`%w

Deletion requires authorization with the "delete_repo" scope. To authorize, run "gh auth refresh -s delete_repo"`, err)
	} else if resp.StatusCode > 204 {
		return err
	}

	return nil
}
