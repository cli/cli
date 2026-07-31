package shared

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
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

	restPrefix := ghinstance.RESTPrefix(repo.RepoHost())
	perPage := 100
	u, err := safeurl.JoinPathWithHostPrefix(restPrefix, "repos", repo.RepoOwner(), repo.RepoName(), "actions", "artifacts")
	if err != nil {
		return nil, err
	}
	if runID != "" {
		u, err = safeurl.JoinPathWithHostPrefix(restPrefix, "repos", repo.RepoOwner(), repo.RepoName(), "actions", "runs", runID, "artifacts")
		if err != nil {
			return nil, err
		}
	}
	u.SetQuery("per_page", strconv.Itoa(perPage))
	var pageURL safeurl.SafeURL = u

	for {
		var payload artifactsPayload
		nextURL, err := apiGet(httpClient, pageURL, &payload)
		if err != nil {
			return nil, err
		}
		results = append(results, payload.Artifacts...)

		if nextURL == "" {
			break
		}
		pageURL = safeurl.NewImmutableSafeURL(nextURL)
	}

	return results, nil
}

func apiGet(httpClient *http.Client, url safeurl.SafeURL, data interface{}) (string, error) {
	req, err := http.NewRequest("GET", url.String(), nil)
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
