package rename

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
)

func apiRename(client *http.Client, repo ghrepo.Interface, newRepoName string) (ghrepo.Interface, error) {
	input := map[string]string{"name": newRepoName}
	body, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}

	path := fmt.Sprintf("%srepos/%s",
		ghinstance.RESTPrefix(repo.RepoHost()),
		ghrepo.FullName(repo))

	request, err := http.NewRequest("PATCH", path, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}

	request.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := client.Do(request)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode > 299 {
		return nil, api.HandleHTTPError(resp)
	}

	defer resp.Body.Close()
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	result := struct {
		Name  string
		Owner struct {
			Login string
		}
	}{}
	if err := json.Unmarshal(b, &result); err != nil {
		return nil, fmt.Errorf("error unmarshaling response: %w", err)
	}

	newRepo := ghrepo.NewWithHost(result.Owner.Login, result.Name, repo.RepoHost())

	return newRepo, nil
}
