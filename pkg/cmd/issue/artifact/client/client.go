// Package client is the REST API client for issue artifacts. Commands reach
// the API through the ArtifactClient interface, so their tests can use the
// generated mock instead of HTTP stubs.
package client

import (
	"bytes"
	"encoding/json"
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
	// UploadTarget is IsPullRequest for a command that uploads files with
	// --attach. Its one lookup also returns what an upload needs to know
	// about repo.
	UploadTarget(repo ghrepo.Interface, number int) (*UploadTarget, error)
	// List returns the artifacts on an issue in the order the API returns
	// them, which is number order. An empty artifactType lists every type,
	// and a limit of 0 lists every artifact.
	List(repo ghrepo.Interface, issueNumber int, artifactType string, limit int) ([]Artifact, error)
	// Get returns one artifact on an issue, with its edit history.
	Get(repo ghrepo.Interface, issueNumber int, number int) (*ArtifactWithVersions, error)
	// Delete deletes one artifact from an issue. The API can't restore a
	// deleted artifact.
	Delete(repo ghrepo.Interface, issueNumber int, number int) error
	// Create creates one artifact on an issue and returns it. The API doesn't
	// return a new artifact's description.
	Create(repo ghrepo.Interface, issueNumber int, artifactType, name, body string) (*Artifact, error)
	// Update saves a new version of one artifact on an issue and returns the
	// artifact. A nil name or body keeps the current one. The API can't change
	// an artifact's type.
	Update(repo ghrepo.Interface, issueNumber int, number int, name, body *string) (*Artifact, error)
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

func (c *artifactClient) UploadTarget(repo ghrepo.Interface, number int) (*UploadTarget, error) {
	issue, err := issueShared.FindIssueOrPR(c.httpClient, repo, number, []string{"number", "repository"})
	if err != nil {
		return nil, err
	}
	return &UploadTarget{
		IsPullRequest:    issue.IsPullRequest(),
		RepositoryID:     issue.RepositoryDatabaseID(),
		ViewerPermission: issue.RepositoryViewerPermission(),
	}, nil
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

func (c *artifactClient) Get(repo ghrepo.Interface, issueNumber int, number int) (*ArtifactWithVersions, error) {
	u, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "issues", strconv.Itoa(issueNumber), "artifacts", strconv.Itoa(number))
	if err != nil {
		return nil, err
	}

	var response struct {
		Artifact Artifact  `json:"artifact"`
		Versions []Version `json:"versions"`
	}
	if err := c.apiClient.REST(repo.RepoHost(), "GET", u.String(), nil, &response); err != nil {
		return nil, err
	}
	return &ArtifactWithVersions{Artifact: response.Artifact, Versions: response.Versions}, nil
}

func (c *artifactClient) Delete(repo ghrepo.Interface, issueNumber int, number int) error {
	u, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "issues", strconv.Itoa(issueNumber), "artifacts", strconv.Itoa(number))
	if err != nil {
		return err
	}
	return c.apiClient.REST(repo.RepoHost(), "DELETE", u.String(), nil, nil)
}

func (c *artifactClient) Create(repo ghrepo.Interface, issueNumber int, artifactType, name, body string) (*Artifact, error) {
	u, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "issues", strconv.Itoa(issueNumber), "artifacts")
	if err != nil {
		return nil, err
	}

	payload, err := json.Marshal(map[string]string{
		"type": artifactType,
		"name": name,
		"body": body,
	})
	if err != nil {
		return nil, err
	}

	var artifact Artifact
	if err := c.apiClient.REST(repo.RepoHost(), "POST", u.String(), bytes.NewReader(payload), &artifact); err != nil {
		return nil, err
	}
	return &artifact, nil
}

// Update sends only the fields it changes, since the API keeps any field a
// request leaves out.
func (c *artifactClient) Update(repo ghrepo.Interface, issueNumber int, number int, name, body *string) (*Artifact, error) {
	u, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "issues", strconv.Itoa(issueNumber), "artifacts", strconv.Itoa(number))
	if err != nil {
		return nil, err
	}

	fields := map[string]string{}
	if name != nil {
		fields["name"] = *name
	}
	if body != nil {
		fields["body"] = *body
	}
	payload, err := json.Marshal(fields)
	if err != nil {
		return nil, err
	}

	var artifact Artifact
	if err := c.apiClient.REST(repo.RepoHost(), "PATCH", u.String(), bytes.NewReader(payload), &artifact); err != nil {
		return nil, err
	}
	return &artifact, nil
}
