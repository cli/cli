package shared

import (
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

	// TODO(api-client-rollout)
	// This line of code is part of a mechanical roll out of the api client.
	// As a follow up, consider whether the api client can be injected to this call site, rather than constructed
	client := api.NewClientFromHTTP(httpClient)

	for {
		var payload artifactsPayload
		nextURL, err := client.RESTWithNext(repo.RepoHost(), http.MethodGet, pageURL.String(), nil, &payload)
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
