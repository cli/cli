package shared

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/githubrest"
	"github.com/cli/cli/v2/internal/safeurl"
)

// repositoryV3 is the repository result from GitHub API v3.
type repositoryV3 struct {
	NodeID    string `json:"node_id"`
	Name      string
	CreatedAt time.Time `json:"created_at"`
	Owner     struct {
		Login string
	}
	Private bool
	HTMLUrl string `json:"html_url"`
	Parent  *repositoryV3
}

// ForkRepo forks the repository on GitHub and returns the new repository.
func ForkRepo(ctx context.Context, client *githubrest.Client, repo ghrepo.Interface, org, newName string, defaultBranchOnly bool) (*api.Repository, error) {
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
	enc := json.NewEncoder(body)
	if err := enc.Encode(params); err != nil {
		return nil, err
	}

	req, err := client.NewRequest(ctx, http.MethodPost, path.String(), body)
	if err != nil {
		return nil, err
	}

	result := repositoryV3{}
	if _, err := client.Do(req, &result); err != nil {
		return nil, err
	}

	newRepo := &api.Repository{
		ID:        result.NodeID,
		Name:      result.Name,
		CreatedAt: result.CreatedAt,
		Owner: api.RepositoryOwner{
			Login: result.Owner.Login,
		},
		ViewerPermission: "WRITE",
	}
	newRepo = api.InitRepoHostname(newRepo, repo.RepoHost())

	// The GitHub API will happily return a HTTP 200 when attempting to fork own repo even though no forking
	// actually took place. Ensure that we raise an error instead.
	if ghrepo.IsSame(repo, newRepo) {
		return newRepo, fmt.Errorf("%s cannot be forked. A single user account cannot own both a parent and fork.", ghrepo.FullName(repo))
	}

	return newRepo, nil
}

// RenameRepo renames the repository on GitHub and returns the renamed repository.
func RenameRepo(ctx context.Context, client *githubrest.Client, repo ghrepo.Interface, newRepoName string) (*api.Repository, error) {
	input := map[string]string{"name": newRepoName}
	body := &bytes.Buffer{}
	enc := json.NewEncoder(body)
	if err := enc.Encode(input); err != nil {
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

	result := repositoryV3{}
	if _, err := client.Do(req, &result); err != nil {
		return nil, err
	}

	renamed := &api.Repository{
		ID:        result.NodeID,
		Name:      result.Name,
		CreatedAt: result.CreatedAt,
		Owner: api.RepositoryOwner{
			Login: result.Owner.Login,
		},
		ViewerPermission: "WRITE",
	}
	return api.InitRepoHostname(renamed, repo.RepoHost()), nil
}

// CreateRepoTransformToV4 issues a REST repository create/edit request and
// transforms the API v3 response into an api.Repository.
func CreateRepoTransformToV4(ctx context.Context, client *githubrest.Client, hostname string, method string, path safeurl.SafeURL, body io.Reader) (*api.Repository, error) {
	req, err := client.NewRequest(ctx, method, path.String(), body)
	if err != nil {
		return nil, err
	}

	var responsev3 repositoryV3
	if _, err := client.Do(req, &responsev3); err != nil {
		return nil, err
	}

	repo := &api.Repository{
		Name:      responsev3.Name,
		CreatedAt: responsev3.CreatedAt,
		Owner: api.RepositoryOwner{
			Login: responsev3.Owner.Login,
		},
		ID:        responsev3.NodeID,
		URL:       responsev3.HTMLUrl,
		IsPrivate: responsev3.Private,
	}
	return api.InitRepoHostname(repo, hostname), nil
}

// RepoLicenses fetches available repository licenses.
// It uses API v3 because licenses are not supported by GraphQL.
func RepoLicenses(ctx context.Context, client *githubrest.Client, hostname string) ([]api.License, error) {
	path, err := safeurl.JoinPath("licenses")
	if err != nil {
		return nil, err
	}
	req, err := client.NewRequest(ctx, http.MethodGet, path.String(), nil)
	if err != nil {
		return nil, err
	}
	var licenses []api.License
	if _, err := client.Do(req, &licenses); err != nil {
		return nil, err
	}
	return licenses, nil
}

// RepoLicense fetches an available repository license.
// It uses API v3 because licenses are not supported by GraphQL.
func RepoLicense(ctx context.Context, client *githubrest.Client, hostname string, licenseName string) (*api.License, error) {
	path, err := safeurl.JoinPath("licenses", licenseName)
	if err != nil {
		return nil, err
	}
	req, err := client.NewRequest(ctx, http.MethodGet, path.String(), nil)
	if err != nil {
		return nil, err
	}
	var license api.License
	if _, err := client.Do(req, &license); err != nil {
		return nil, err
	}
	return &license, nil
}

// RepoGitIgnoreTemplates fetches available repository gitignore templates.
// It uses API v3 here because gitignore template isn't supported by GraphQL.
func RepoGitIgnoreTemplates(ctx context.Context, client *githubrest.Client, hostname string) ([]string, error) {
	path, err := safeurl.JoinPath("gitignore", "templates")
	if err != nil {
		return nil, err
	}
	req, err := client.NewRequest(ctx, http.MethodGet, path.String(), nil)
	if err != nil {
		return nil, err
	}
	var gitIgnoreTemplates []string
	if _, err := client.Do(req, &gitIgnoreTemplates); err != nil {
		return nil, err
	}
	return gitIgnoreTemplates, nil
}

// RepoGitIgnoreTemplate fetches an available repository gitignore template.
// It uses API v3 here because gitignore template isn't supported by GraphQL.
func RepoGitIgnoreTemplate(ctx context.Context, client *githubrest.Client, hostname string, gitIgnoreTemplateName string) (*api.GitIgnore, error) {
	path, err := safeurl.JoinPath("gitignore", "templates", gitIgnoreTemplateName)
	if err != nil {
		return nil, err
	}
	req, err := client.NewRequest(ctx, http.MethodGet, path.String(), nil)
	if err != nil {
		return nil, err
	}
	var gitIgnoreTemplate api.GitIgnore
	if _, err := client.Do(req, &gitIgnoreTemplate); err != nil {
		return nil, err
	}
	return &gitIgnoreTemplate, nil
}
