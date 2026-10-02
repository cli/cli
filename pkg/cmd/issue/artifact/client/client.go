// Package client is the REST API client for issue artifacts. Commands reach
// the API through the ArtifactClient interface, so their tests can use the
// generated mock instead of HTTP stubs.
package client

import (
	"net/http"
	"strconv"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
	issueShared "github.com/cli/cli/v2/pkg/cmd/issue/shared"
)

//go:generate moq -rm -out client_mock.go . ArtifactClient

// ArtifactClient is how the artifact commands reach the API. Every request
// goes to the host of the repository it is given.
type ArtifactClient interface {
	// IsPullRequest reports whether number in repo is a pull request rather
	// than an issue, with one lookup.
	IsPullRequest(repo ghrepo.Interface, number int) (bool, error)
	// List returns the artifacts on an issue in the order the API returns
	// them, which is number order. An empty artifactType lists every type,
	// and a limit of 0 lists every artifact.
	List(repo ghrepo.Interface, issueNumber int, artifactType string, limit int) ([]Artifact, error)
}

// maxPageSize is the most artifacts one request asks for.
const maxPageSize = 100

type artifactClient struct {
	httpClient *http.Client
	apiClient  *api.Client
}

// NewArtifactClient returns an ArtifactClient that sends its requests through
// httpClient.
func NewArtifactClient(httpClient *http.Client) ArtifactClient {
	return &artifactClient{
		httpClient: httpClient,
		apiClient:  api.NewClientFromHTTP(httpClient),
	}
}

func (c *artifactClient) IsPullRequest(repo ghrepo.Interface, number int) (bool, error) {
	issue, err := issueShared.FindIssueOrPR(c.httpClient, repo, number, []string{"number"})
	if err != nil {
		return false, err
	}
	return issue.IsPullRequest(), nil
}

// List requests pages until one comes back short or the limit is reached.
// Every request asks for the same page size, so the page numbers line up.
func (c *artifactClient) List(repo ghrepo.Interface, issueNumber int, artifactType string, limit int) ([]Artifact, error) {
	u, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "issues", strconv.Itoa(issueNumber), "artifacts")
	if err != nil {
		return nil, err
	}

	perPage := maxPageSize
	if limit > 0 && limit < maxPageSize {
		perPage = limit
	}
	u.SetQuery("per_page", strconv.Itoa(perPage))
	if artifactType != "" {
		u.SetQuery("type", artifactType)
	}

	artifacts := []Artifact{}
	for page := 1; ; page++ {
		if page > 1 {
			u.SetQuery("page", strconv.Itoa(page))
		}

		var pageArtifacts []Artifact
		if err := c.apiClient.REST(repo.RepoHost(), "GET", u.String(), nil, &pageArtifacts); err != nil {
			return nil, err
		}
		artifacts = append(artifacts, pageArtifacts...)

		if len(pageArtifacts) < perPage || (limit > 0 && len(artifacts) >= limit) {
			break
		}
	}

	if limit > 0 && len(artifacts) > limit {
		artifacts = artifacts[:limit]
	}
	return artifacts, nil
}
