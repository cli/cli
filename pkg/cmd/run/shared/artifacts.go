package shared

import (
	"context"
	"github.com/cli/cli/v2/internal/githubrest"
	"net/http"
	"strconv"

	"github.com/cli/cli/v2/api"
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

	client, err := api.NewRESTClient(httpClient, repo.RepoHost())
	if err != nil {
		return nil, err
	}

	perPage := 100
	u, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "actions", "artifacts")
	if err != nil {
		return nil, err
	}
	if runID != "" {
		u, err = safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "actions", "runs", runID, "artifacts")
		if err != nil {
			return nil, err
		}
	}
	u.SetQuery("per_page", strconv.Itoa(perPage))
	var pageURL safeurl.SafeURL = u

	for {
		var payload artifactsPayload
		nextURL, err := apiGet(client, pageURL, &payload)
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

func apiGet(client *githubrest.Client, url safeurl.SafeURL, data interface{}) (string, error) {
	req, err := client.NewRequest(context.Background(), http.MethodGet, url.String(), nil)
	if err != nil {
		return "", err
	}

	resp, err := client.Do(req, data)
	if err != nil {
		return "", err
	}

	return resp.NextPage(), nil
}
