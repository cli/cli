package shared

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"

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

type artifactsPayload struct {
	Artifacts []Artifact
}

func ListArtifacts(httpClient *http.Client, repo ghrepo.Interface, runID string) ([]Artifact, error) {
	var results []Artifact

	perPage := 100
	path := fmt.Sprintf("repos/%s/%s/actions/artifacts?per_page=%d", repo.RepoOwner(), repo.RepoName(), perPage)
	if runID != "" {
		path = fmt.Sprintf("repos/%s/%s/actions/runs/%s/artifacts?per_page=%d", repo.RepoOwner(), repo.RepoName(), runID, perPage)
	}

	url := fmt.Sprintf("%s%s", ghinstance.RESTPrefix(repo.RepoHost()), path)

	for {
		var payload artifactsPayload
		nextURL, err := apiGet(httpClient, url, &payload)
		if err != nil {
			return nil, err
		}
		results = append(results, payload.Artifacts...)

		if nextURL == "" {
			break
		}
		url = nextURL
	}

	return results, nil
}

func apiGet(httpClient *http.Client, url string, data interface{}) (string, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode > 299 {
		return "", api.HandleHTTPError(resp)
	}

	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(data); err != nil {
		return "", err
	}

	return findNextPage(resp), nil
}

var linkRE = regexp.MustCompile(`<([^>]+)>;\s*rel="([^"]+)"`)

func findNextPage(resp *http.Response) string {
	for _, m := range linkRE.FindAllStringSubmatch(resp.Header.Get("Link"), -1) {
		if len(m) > 2 && m[2] == "next" {
			return m[1]
		}
	}
	return ""
}
