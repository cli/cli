package rename

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
)

type renameRepo struct {
	RepoHost  string
	RepoOwner string
	RepoName  string
	Name      string `json:"name,omitempty"`
}

func runRename(client *http.Client, repo ghrepo.Interface, newRepoName string) error {

	input := renameRepo{
		RepoHost:  repo.RepoHost(),
		RepoOwner: repo.RepoOwner(),
		RepoName:  repo.RepoName(),
		Name:      newRepoName,
	}

	body, err := json.Marshal(input)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("%srepos/%s",
		ghinstance.RESTPrefix(repo.RepoHost()),
		ghrepo.FullName(repo))

	request, err := http.NewRequest("PATCH", path, bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode > 299 {
		return api.HandleHTTPError(response)
	}
	return nil
}
