package list

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
)

var scopesError = errors.New("insufficient OAuth scopes")

type deployKey struct {
	ID        int
	Key       string
	Title     string
	CreatedAt time.Time `json:"created_at"`
	ReadOnly  bool      `json:"read_only"`
}

func repoKeys(httpClient *http.Client, host string, repo ghrepo.Interface) ([]deployKey, error) {
	resource := fmt.Sprintf("repos/%s/%s/keys", repo.RepoOwner(), repo.RepoName())
	url := fmt.Sprintf("%s%s?per_page=%d", ghinstance.RESTPrefix(host), resource, 100)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, scopesError
	} else if resp.StatusCode > 299 {
		return nil, api.HandleHTTPError(resp)
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var keys []deployKey
	err = json.Unmarshal(b, &keys)
	if err != nil {
		return nil, err
	}

	return keys, nil
}
