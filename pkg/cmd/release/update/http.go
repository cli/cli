package update

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/release/shared"
	"io/ioutil"
	"net/http"
)

func updateRelease(httpClient *http.Client, repo ghrepo.Interface, releaseId string, params map[string]interface{}) (*shared.Release, error) {
	bodyBytes, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	path := fmt.Sprintf("repos/%s/%s/releases/%s", repo.RepoOwner(), repo.RepoName(), releaseId)
	url := ghinstance.RESTPrefix(repo.RepoHost()) + path
	req, err := http.NewRequest("PATCH", url, bytes.NewBuffer(bodyBytes))
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
