package create

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/release/shared"
)

var notImplementedError = errors.New("not implemented")

type ReleaseNotes struct {
	Name string `json:"name"`
	Body string `json:"body"`
}

func generateReleaseNotes(httpClient *http.Client, repo ghrepo.Interface, params map[string]interface{}) (*ReleaseNotes, error) {
	bodyBytes, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	path := fmt.Sprintf("repos/%s/%s/releases/generate-notes", repo.RepoOwner(), repo.RepoName())
	url := ghinstance.RESTPrefix(repo.RepoHost()) + path
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, notImplementedError
	}

	success := resp.StatusCode >= 200 && resp.StatusCode < 300
	if !success {
		return nil, api.HandleHTTPError(resp)
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var releaseNotes ReleaseNotes
	err = json.Unmarshal(b, &releaseNotes)
	return &releaseNotes, err
}

func createRelease(httpClient *http.Client, repo ghrepo.Interface, params map[string]interface{}) (*shared.Release, error) {
	bodyBytes, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	path := fmt.Sprintf("repos/%s/%s/releases", repo.RepoOwner(), repo.RepoName())
	url := ghinstance.RESTPrefix(repo.RepoHost()) + path
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	success := resp.StatusCode >= 200 && resp.StatusCode < 300
	if !success {
		return nil, api.HandleHTTPError(resp)
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var newRelease shared.Release
	err = json.Unmarshal(b, &newRelease)
	return &newRelease, err
}

func publishRelease(httpClient *http.Client, releaseURL string) (*shared.Release, error) {
	req, err := http.NewRequest("PATCH", releaseURL, bytes.NewBufferString(`{"draft":false}`))
	if err != nil {
		return nil, err
	}

	req.Header.Add("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	if resp.StatusCode > 299 {
		return nil, api.HandleHTTPError(resp)
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var release shared.Release
	err = json.Unmarshal(b, &release)
	return &release, err
}
