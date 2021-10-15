package shared

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
)

type Artifact struct {
	Name        string `json:"name"`
	Size        uint64 `json:"size_in_bytes"`
	DownloadURL string `json:"archive_download_url"`
	Expired     bool   `json:"expired"`
}

func ListArtifacts(httpClient *http.Client, repo ghrepo.Interface, runID string) ([]Artifact, error) {
	perPage := 100
	path := fmt.Sprintf("repos/%s/%s/actions/artifacts?per_page=%d", repo.RepoOwner(), repo.RepoName(), perPage)
	if runID != "" {
		path = fmt.Sprintf("repos/%s/%s/actions/runs/%s/artifacts?per_page=%d", repo.RepoOwner(), repo.RepoName(), runID, perPage)
	}

	req, err := http.NewRequest("GET", ghinstance.RESTPrefix(repo.RepoHost())+path, nil)
	if err != nil {
		return nil, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode > 299 {
		return nil, api.HandleHTTPError(resp)
	}

	var response struct {
		TotalCount uint16 `json:"total_count"`
		Artifacts  []Artifact
	}

	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&response); err != nil {
		return response.Artifacts, fmt.Errorf("error parsing JSON: %w", err)
	}

	return response.Artifacts, nil
}
