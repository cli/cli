package create

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
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

func getTags(httpClient *http.Client, repo ghrepo.Interface, limit int) ([]tag, error) {
	path := fmt.Sprintf("repos/%s/%s/tags?per_page=%d", repo.RepoOwner(), repo.RepoName(), limit)
	url := ghinstance.RESTPrefix(repo.RepoHost()) + path
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	success := resp.StatusCode >= 200 && resp.StatusCode < 300
	if !success {
		return nil, api.HandleHTTPError(resp)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var tags []tag
	err = json.Unmarshal(b, &tags)
	return tags, err
}

func generateReleaseNotes(httpClient *http.Client, repo ghrepo.Interface, tagName, target, previousTagName string) (*releaseNotes, error) {
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

	path := fmt.Sprintf("repos/%s/%s/releases/generate-notes", repo.RepoOwner(), repo.RepoName())
	url := ghinstance.RESTPrefix(repo.RepoHost()) + path
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, notImplementedError
	}

	success := resp.StatusCode >= 200 && resp.StatusCode < 300
	if !success {
		return nil, api.HandleHTTPError(resp)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var rn releaseNotes
	err = json.Unmarshal(b, &rn)
	return &rn, err
}

func publishedReleaseExists(httpClient *http.Client, repo ghrepo.Interface, tagName string) (bool, error) {
	path := fmt.Sprintf("repos/%s/%s/releases/tags/%s", repo.RepoOwner(), repo.RepoName(), url.PathEscape(tagName))
	url := ghinstance.RESTPrefix(repo.RepoHost()) + path
	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return false, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return false, err
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}

	if resp.StatusCode == 200 {
		return true, nil
	} else if resp.StatusCode == 404 {
		return false, nil
	} else {
		return false, api.HandleHTTPError(resp)
	}
}

func createRelease(httpClient *http.Client, repo ghrepo.Interface, params map[string]interface{}) (*shared.Release, error) {
	bodyBytes, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	path := fmt.Sprintf("repos/%s/%s/releases", repo.RepoOwner(), repo.RepoName())
	url := ghinstance.RESTPrefix(repo.RepoHost()) + path
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

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
	if resp.StatusCode == http.StatusNotFound && !tokenHasWorkflowScope(resp) {
		normalizedHostname := ghauth.NormalizeHostname(resp.Request.URL.Hostname())
		return nil, &errMissingRequiredWorkflowScope{
			Hostname: normalizedHostname,
		}
	}

	success := resp.StatusCode >= 200 && resp.StatusCode < 300
	if !success {
		return nil, api.HandleHTTPError(resp)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var newRelease shared.Release
	err = json.Unmarshal(b, &newRelease)
	return &newRelease, err
}

func publishRelease(httpClient *http.Client, releaseURL string, discussionCategory string, isLatest *bool) (*shared.Release, error) {
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
	req, err := http.NewRequest("PATCH", releaseURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, err
	}

	req.Header.Add("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	if resp.StatusCode > 299 {
		return nil, api.HandleHTTPError(resp)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var release shared.Release
	err = json.Unmarshal(b, &release)
	return &release, err
}

func deleteRelease(httpClient *http.Client, release *shared.Release) error {
	req, err := http.NewRequest("DELETE", release.APIURL, nil)
	if err != nil {
		return err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}

	success := resp.StatusCode >= 200 && resp.StatusCode < 300
	if !success {
		return api.HandleHTTPError(resp)
	}

	if resp.StatusCode != 204 {
		_, _ = io.Copy(io.Discard, resp.Body)
	}
	return nil
}

// tokenHasWorkflowScope checks if the given http.Response's token has the workflow scope.
// Tokens that do not have OAuth scopes are assumed to have the workflow scope.
func tokenHasWorkflowScope(resp *http.Response) bool {
	scopes := resp.Header.Get("X-Oauth-Scopes")

	// Return true when no scopes are present - no scopes in this header
	// means that the user is probably authenticating with a token type other
	// than an OAuth token, and we don't know what this token's scopes actually are.
	if scopes == "" {
		return true
	}

	return slices.Contains(strings.Split(scopes, ","), "workflow")
}
