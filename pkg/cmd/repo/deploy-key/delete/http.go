package delete

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

	if resp.StatusCode == 404 {
		return scopesError
	} else if resp.StatusCode > 299 {
		return api.HandleHTTPError(resp)
	}

	_, err = io.Copy(ioutil.Discard, resp.Body)
	if err != nil {
		return err
	}

	return nil
}

func deployKeyByID(httpClient *http.Client, repo ghrepo.Interface, ID string) (*deployKey, error) {
	path := fmt.Sprintf("repos/%s/%s/keys/%s", repo.RepoOwner(), repo.RepoName(), ID)
	url := ghinstance.RESTPrefix(repo.RepoHost()) + path

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

	key := new(deployKey)
	err = json.Unmarshal(b, key)
	if err != nil {
		return nil, err
	}

	return key, nil
}
