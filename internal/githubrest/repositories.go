package githubrest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
)

// Repository is a repository as returned by the REST API.
//
// It is deliberately separate from api.Repository, which models the far larger
// GraphQL schema. Callers that need the GraphQL shape map between the two at
// the point where they need it, rather than this package depending on api.
type Repository struct {
	NodeID    string    `json:"node_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	Owner     struct {
		Login string `json:"login"`
	} `json:"owner"`
	Private bool        `json:"private"`
	HTMLURL string      `json:"html_url"`
	Parent  *Repository `json:"parent"`

	// Hostname is not part of the API response. It is filled in from the host
	// the request was made against so that Repository satisfies ghrepo.Interface.
	Hostname string `json:"-"`
}

func (r Repository) RepoName() string  { return r.Name }
func (r Repository) RepoOwner() string { return r.Owner.Login }
func (r Repository) RepoHost() string  { return r.Hostname }

// setHostname records the host a repository was fetched from, including on the
// parent, which the API returns without any indication of its host.
func (r *Repository) setHostname(hostname string) *Repository {
	r.Hostname = hostname
	if r.Parent != nil {
		r.Parent.Hostname = hostname
	}
	return r
}

// License is a license as returned by the REST API. Licenses have no GraphQL
// equivalent, which is why this endpoint exists at all.
type License struct {
	Key            string   `json:"key"`
	Name           string   `json:"name"`
	SPDXID         string   `json:"spdx_id"`
	URL            string   `json:"url"`
	NodeID         string   `json:"node_id"`
	HTMLURL        string   `json:"html_url"`
	Description    string   `json:"description"`
	Implementation string   `json:"implementation"`
	Permissions    []string `json:"permissions"`
	Conditions     []string `json:"conditions"`
	Limitations    []string `json:"limitations"`
	Body           string   `json:"body"`
	Featured       bool     `json:"featured"`
}

// GitIgnore is a gitignore template as returned by the REST API. Templates have
// no GraphQL equivalent.
type GitIgnore struct {
	Name   string `json:"name"`
	Source string `json:"source"`
}

// ForkRepo forks the repository on GitHub and returns the new repository.
func ForkRepo(ctx context.Context, client *Client, repo ghrepo.Interface, org, newName string, defaultBranchOnly bool) (*Repository, error) {
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "forks")
	if err != nil {
		return nil, err
	}

	params := map[string]interface{}{}
	if org != "" {
		params["organization"] = org
	}
	if newName != "" {
		params["name"] = newName
	}
	if defaultBranchOnly {
		params["default_branch_only"] = true
	}

	body := &bytes.Buffer{}
	if err := json.NewEncoder(body).Encode(params); err != nil {
		return nil, err
	}

	req, err := client.NewRequest(ctx, http.MethodPost, path.String(), body)
	if err != nil {
		return nil, err
	}

	var forked Repository
	if _, err := client.Do(req, &forked); err != nil {
		return nil, err
	}
	forked.setHostname(repo.RepoHost())

	// The GitHub API will happily return a HTTP 200 when attempting to fork own repo even though no forking
	// actually took place. Ensure that we raise an error instead.
	if ghrepo.IsSame(repo, &forked) {
		return &forked, fmt.Errorf("%s cannot be forked. A single user account cannot own both a parent and fork.", ghrepo.FullName(repo))
	}

	return &forked, nil
}

// RenameRepo renames the repository on GitHub and returns the renamed repository.
func RenameRepo(ctx context.Context, client *Client, repo ghrepo.Interface, newRepoName string) (*Repository, error) {
	body := &bytes.Buffer{}
	if err := json.NewEncoder(body).Encode(map[string]string{"name": newRepoName}); err != nil {
		return nil, err
	}

	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName())
	if err != nil {
		return nil, err
	}

	req, err := client.NewRequest(ctx, http.MethodPatch, path.String(), body)
	if err != nil {
		return nil, err
	}

	var renamed Repository
	if _, err := client.Do(req, &renamed); err != nil {
		return nil, err
	}
	return renamed.setHostname(repo.RepoHost()), nil
}

// CreateRepo creates a repository. The caller supplies the path because the
// endpoint differs between user-owned and organization-owned repositories.
func CreateRepo(ctx context.Context, client *Client, hostname string, path safeurl.SafeURL, body io.Reader) (*Repository, error) {
	req, err := client.NewRequest(ctx, http.MethodPost, path.String(), body)
	if err != nil {
		return nil, err
	}

	var created Repository
	if _, err := client.Do(req, &created); err != nil {
		return nil, err
	}
	return created.setHostname(hostname), nil
}

// EditRepo applies a partial update to a repository. The response is discarded
// because no caller needs it.
func EditRepo(ctx context.Context, client *Client, repo ghrepo.Interface, body io.Reader) error {
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName())
	if err != nil {
		return err
	}
	req, err := client.NewRequest(ctx, http.MethodPatch, path.String(), body)
	if err != nil {
		return err
	}
	_, err = client.Do(req, nil)
	return err
}

// BranchDeleteRemote deletes a branch ref on the given repository.
func BranchDeleteRemote(ctx context.Context, client *Client, repo ghrepo.Interface, branch string) error {
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "git", "refs", fmt.Sprintf("heads/%s", branch))
	if err != nil {
		return err
	}
	req, err := client.NewRequest(ctx, http.MethodDelete, path.String(), nil)
	if err != nil {
		return err
	}
	_, err = client.Do(req, nil)
	return err
}

// RepoLicenses fetches the licenses GitHub can apply to a new repository.
func RepoLicenses(ctx context.Context, client *Client) ([]License, error) {
	path, err := safeurl.JoinPath("licenses")
	if err != nil {
		return nil, err
	}
	req, err := client.NewRequest(ctx, http.MethodGet, path.String(), nil)
	if err != nil {
		return nil, err
	}
	var licenses []License
	if _, err := client.Do(req, &licenses); err != nil {
		return nil, err
	}
	return licenses, nil
}

// RepoLicense fetches a single license by its key.
func RepoLicense(ctx context.Context, client *Client, licenseName string) (*License, error) {
	path, err := safeurl.JoinPath("licenses", licenseName)
	if err != nil {
		return nil, err
	}
	req, err := client.NewRequest(ctx, http.MethodGet, path.String(), nil)
	if err != nil {
		return nil, err
	}
	var license License
	if _, err := client.Do(req, &license); err != nil {
		return nil, err
	}
	return &license, nil
}

// RepoGitIgnoreTemplates fetches the names of the available gitignore templates.
func RepoGitIgnoreTemplates(ctx context.Context, client *Client) ([]string, error) {
	path, err := safeurl.JoinPath("gitignore", "templates")
	if err != nil {
		return nil, err
	}
	req, err := client.NewRequest(ctx, http.MethodGet, path.String(), nil)
	if err != nil {
		return nil, err
	}
	var templates []string
	if _, err := client.Do(req, &templates); err != nil {
		return nil, err
	}
	return templates, nil
}

// RepoGitIgnoreTemplate fetches a single gitignore template by name.
func RepoGitIgnoreTemplate(ctx context.Context, client *Client, name string) (*GitIgnore, error) {
	path, err := safeurl.JoinPath("gitignore", "templates", name)
	if err != nil {
		return nil, err
	}
	req, err := client.NewRequest(ctx, http.MethodGet, path.String(), nil)
	if err != nil {
		return nil, err
	}
	var template GitIgnore
	if _, err := client.Do(req, &template); err != nil {
		return nil, err
	}
	return &template, nil
}
