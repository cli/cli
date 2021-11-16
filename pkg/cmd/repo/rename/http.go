package rename

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/go-gh/pkg/api"
)

func apiRename(client api.RESTClient, repo ghrepo.Interface, newRepoName string) (ghrepo.Interface, error) {
	input := map[string]string{"name": newRepoName}
	body, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}

	path := fmt.Sprintf("repos/%s", ghrepo.FullName(repo))

	result := struct {
		Name  string
		Owner struct {
			Login string
		}
	}{}

	err = client.Patch(path, bytes.NewReader(body), result)
	if err != nil {
		return nil, err
	}

	newRepo := ghrepo.NewWithHost(result.Owner.Login, result.Name, repo.RepoHost())

	return newRepo, nil
}
