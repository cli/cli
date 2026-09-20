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
	"github.com/cli/cli/v2/internal/ghrepo"
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
	variables := map[string]any{
		"owner":   githubv4.String(repo.RepoOwner()),
		"name":    githubv4.String(repo.RepoName()),
		"tagName": githubv4.String(qualifiedTagName),
	}
	err := gql.Query(repo.RepoHost(), "RepositoryFindRef", &query, variables)
	return query.Repository.Ref.ID != "", err
}

func getTags(httpClient *http.Client, repo ghrepo.Interface, limit int) ([]tag, error) {
	u, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "tags")
	if err != nil {
		return nil, err
	}
	u.SetQuery("per_page", strconv.Itoa(limit))

	var tags []tag
	// TODO(api-client-rollout)
	// This line of code is part of a mechanical roll out of the api client.
	// As a follow up, consider whether the api client can be injected to this call site, rather than constructed
	err = api.NewClientFromHTTP(httpClient).REST(repo.RepoHost(), http.MethodGet, u.String(), nil, &tags)
	return tags, err
}

func generateReleaseNotes(httpClient *http.Client, repo ghrepo.Interface, tagName, target, previousTagName string) (*releaseNotes, error) {
	params := map[string]any{
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

	var rn releaseNotes
	// TODO(api-client-rollout)
	// This line of code is part of a mechanical roll out of the api client.
	// As a follow up, consider whether the api client can be injected to this call site, rather than constructed
	err = api.NewClientFromHTTP(httpClient).REST(repo.RepoHost(), http.MethodPost, path.String(), bytes.NewBuffer(bodyBytes), &rn)
	if err != nil {
		if httpErr, ok := errors.AsType[api.HTTPError](err); ok && httpErr.StatusCode == http.StatusNotFound {
			return nil, notImplementedError
		}
		return nil, err
	}
	return &rn, nil
}

func publishedReleaseExists(httpClient *http.Client, repo ghrepo.Interface, tagName string) (bool, error) {
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "releases", "tags", tagName)
	if err != nil {
		return false, err
	}

	// A HEAD response has no body, so Request is used rather than REST, which would try to
	// decode one.
	// TODO(api-client-rollout)
	// This line of code is part of a mechanical roll out of the api client.
	// As a follow up, consider whether the api client can be injected to this call site, rather than constructed
	resp, err := api.NewClientFromHTTP(httpClient).Request(repo.RepoHost(), http.MethodHead, path.String(), nil)
	if err != nil {
		var httpErr api.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			return false, nil
		}
		return false, err
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}

	if resp.StatusCode != http.StatusOK {
		return false, api.UnexpectedStatusError(resp)
	}

	return true, nil
}

func createRelease(httpClient *http.Client, repo ghrepo.Interface, params map[string]any) (*shared.Release, error) {
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
	var newRelease shared.Release
	// TODO(api-client-rollout)
	// This line of code is part of a mechanical roll out of the api client.
	// As a follow up, consider whether the api client can be injected to this call site, rather than constructed
	err = api.NewClientFromHTTP(httpClient).REST(repo.RepoHost(), http.MethodPost, path.String(), bytes.NewBuffer(bodyBytes), &newRelease)
	if err != nil {
		var httpErr api.HTTPError
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

func publishRelease(httpClient *http.Client, host string, releaseURL safeurl.SafeURL, discussionCategory string, isLatest *bool) (*shared.Release, error) {
	params := map[string]any{"draft": false}
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

	var release shared.Release
	// TODO(api-client-rollout)
	// This line of code is part of a mechanical roll out of the api client.
	// As a follow up, consider whether the api client can be injected to this call site, rather than constructed
	err = api.NewClientFromHTTP(httpClient).REST(host, http.MethodPatch, releaseURL.String(), bytes.NewBuffer(bodyBytes), &release)
	if err != nil {
		return nil, err
	}
	return &release, nil
}

func deleteRelease(httpClient *http.Client, host string, releaseURL safeurl.SafeURL) error {
	// TODO(api-client-rollout)
	// This line of code is part of a mechanical roll out of the api client.
	// As a follow up, consider whether the api client can be injected to this call site, rather than constructed
	return api.NewClientFromHTTP(httpClient).REST(host, http.MethodDelete, releaseURL.String(), nil, nil)
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
	for s := range strings.SplitSeq(scopes, ",") {
		if strings.TrimSpace(s) == "workflow" {
			return true
		}
	}
	return false
}

// isNewRelease checks if there are new commits since the latest release.
func isNewRelease(httpClient *http.Client, repo ghrepo.Interface) (bool, error) {
	ctx := context.Background()
	release, err := shared.FetchLatestRelease(ctx, httpClient, repo)
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

	apiClient := api.NewClientFromHTTP(httpClient)
	if err := apiClient.REST(repo.RepoHost(), "GET", u.String(), nil, &comparisonStatus); err != nil {
		return false, err
	}

	isNew := comparisonStatus.Status == "ahead"
	return isNew, nil
}
