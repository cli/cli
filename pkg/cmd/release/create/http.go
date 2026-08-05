package create

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/githubrest"
	"github.com/cli/cli/v2/internal/safeurl"
	"github.com/cli/cli/v2/pkg/cmd/release/shared"
	"github.com/shurcooL/githubv4"

	ghauth "github.com/cli/go-gh/v2/pkg/auth"
)

type tag struct {
	Name string `json:"name"`
}

type releaseNotes struct {
	Name string `json:"name"`
	Body string `json:"body"`
}

var notImplementedError = errors.New("not implemented")

type errMissingRequiredWorkflowScope struct {
	Hostname string
}

func (e errMissingRequiredWorkflowScope) Error() string {
	return "workflow scope may be required"
}

func remoteTagExists(httpClient *http.Client, repo ghrepo.Interface, tagName string) (bool, error) {
	gql := api.NewClientFromHTTP(httpClient)
	qualifiedTagName := fmt.Sprintf("refs/tags/%s", tagName)
	var query struct {
		Repository struct {
			Ref struct {
				ID string
			} `graphql:"ref(qualifiedName: $tagName)"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}
	variables := map[string]interface{}{
		"owner":   githubv4.String(repo.RepoOwner()),
		"name":    githubv4.String(repo.RepoName()),
		"tagName": githubv4.String(qualifiedTagName),
	}
	err := gql.Query(repo.RepoHost(), "RepositoryFindRef", &query, variables)
	return query.Repository.Ref.ID != "", err
}

func getTags(ctx context.Context, client *githubrest.Client, repo ghrepo.Interface, limit int) ([]tag, error) {
	u, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "tags")
	if err != nil {
		return nil, err
	}
	u.SetQuery("per_page", strconv.Itoa(limit))

	req, err := client.NewRequest(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}

	var tags []tag
	_, err = client.Do(req, &tags)
	return tags, err
}

func generateReleaseNotes(ctx context.Context, client *githubrest.Client, repo ghrepo.Interface, tagName, target, previousTagName string) (*releaseNotes, error) {
	params := map[string]interface{}{
		"tag_name": tagName,
	}
	if target != "" {
		params["target_commitish"] = target
	}
	if previousTagName != "" {
		params["previous_tag_name"] = previousTagName
	}

	bodyBytes, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "releases", "generate-notes")
	if err != nil {
		return nil, err
	}

	req, err := client.NewRequest(ctx, http.MethodPost, path.String(), bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, err
	}

	var rn releaseNotes
	if _, err := client.Do(req, &rn); err != nil {
		var httpErr *githubrest.ErrorResponse
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			return nil, notImplementedError
		}
		return nil, err
	}
	return &rn, nil
}

// publishedReleaseExists asks with HEAD, so there is no body to decode; a nil
// decode target says exactly that.
func publishedReleaseExists(ctx context.Context, client *githubrest.Client, repo ghrepo.Interface, tagName string) (bool, error) {
	url, err := safeurl.JoinPathWithHostPrefix(ghinstance.RESTPrefix(repo.RepoHost()), "repos", repo.RepoOwner(), repo.RepoName(), "releases", "tags", tagName)
	if err != nil {
		return false, err
	}
	req, err := client.NewRequest(ctx, http.MethodHead, url.String(), nil)
	if err != nil {
		return false, err
	}

	if _, err := client.Do(req, nil); err != nil {
		var errResp *githubrest.ErrorResponse
		if errors.As(err, &errResp) && errResp.StatusCode == http.StatusNotFound {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

func createRelease(ctx context.Context, client *githubrest.Client, repo ghrepo.Interface, params map[string]interface{}) (*shared.Release, error) {
	bodyBytes, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "releases")
	if err != nil {
		return nil, err
	}

	// Check if we received a 404 while attempting to create a release without
	// the workflow scope, and if so, return an error message that explains a possible
	// solution to the user.
	//
	// If the same file (with both the same path and contents) exists
	// on another branch in the repo, releases with workflow file changes can be
	// created without the workflow scope. Otherwise, the workflow scope is
	// required to create the release, but the API does not indicate this criteria
	// beyond returning a 404.
	//
	// https://docs.github.com/en/apps/oauth-apps/building-oauth-apps/scopes-for-oauth-apps#available-scopes
	req, err := client.NewRequest(ctx, http.MethodPost, path.String(), bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, err
	}

	var newRelease shared.Release
	if _, err := client.Do(req, &newRelease); err != nil {
		var httpErr *githubrest.ErrorResponse
		if errors.As(err, &httpErr) &&
			httpErr.StatusCode == http.StatusNotFound &&
			!tokenHasWorkflowScope(httpErr.Headers) {
			normalizedHostname := ghauth.NormalizeHostname(httpErr.RequestURL.Hostname())
			return nil, &errMissingRequiredWorkflowScope{
				Hostname: normalizedHostname,
			}
		}
		return nil, err
	}
	return &newRelease, nil
}

func publishRelease(ctx context.Context, client *githubrest.Client, releaseURL safeurl.SafeURL, discussionCategory string, isLatest *bool) (*shared.Release, error) {
	params := map[string]interface{}{"draft": false}
	if discussionCategory != "" {
		params["discussion_category_name"] = discussionCategory
	}

	if isLatest != nil {
		params["make_latest"] = fmt.Sprintf("%v", *isLatest)
	}

	bodyBytes, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	req, err := client.NewRequest(ctx, http.MethodPatch, releaseURL.String(), bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, err
	}

	var release shared.Release
	if _, err := client.Do(req, &release); err != nil {
		return nil, err
	}
	return &release, nil
}

func deleteRelease(ctx context.Context, client *githubrest.Client, releaseURL safeurl.SafeURL) error {
	req, err := client.NewRequest(ctx, http.MethodDelete, releaseURL.String(), nil)
	if err != nil {
		return err
	}

	_, err = client.Do(req, nil)
	return err
}

// tokenHasWorkflowScope checks if the response token has the workflow scope.
// Tokens that do not have OAuth scopes are assumed to have the workflow scope.
func tokenHasWorkflowScope(headers http.Header) bool {
	scopes := headers.Get("X-Oauth-Scopes")

	// Return true when no scopes are present - no scopes in this header
	// means that the user is probably authenticating with a token type other
	// than an OAuth token, and we don't know what this token's scopes actually are.
	if scopes == "" {
		return true
	}

	// The API returns scopes separated by a comma and a space, so each element
	// must be trimmed before comparison.
	for _, s := range strings.Split(scopes, ",") {
		if strings.TrimSpace(s) == "workflow" {
			return true
		}
	}
	return false
}

// isNewRelease checks if there are new commits since the latest release.
func isNewRelease(ctx context.Context, client *githubrest.Client, repo ghrepo.Interface) (bool, error) {
	release, err := shared.FetchLatestRelease(ctx, client, repo)
	if err != nil {
		if errors.Is(err, shared.ErrReleaseNotFound) {
			return true, nil
		} else {
			return false, err
		}
	}

	tagName := release.TagName
	u, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "compare", tagName+"...HEAD")
	if err != nil {
		return false, err
	}
	u.SetQuery("per_page", "1")

	var comparisonStatus struct {
		Status string `json:"status"`
	}

	req, err := client.NewRequest(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return false, err
	}

	if _, err := client.Do(req, &comparisonStatus); err != nil {
		return false, err
	}

	isNew := comparisonStatus.Status == "ahead"
	return isNew, nil
}
