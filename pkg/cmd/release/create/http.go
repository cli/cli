package create

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/cli/cli/v2/internal/githubrest"
	"io"
	"net/http"
	"slices"
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
	variables := map[string]interface{}{
		"owner":   githubv4.String(repo.RepoOwner()),
		"name":    githubv4.String(repo.RepoName()),
		"tagName": githubv4.String(qualifiedTagName),
	}
	err := gql.Query(repo.RepoHost(), "RepositoryFindRef", &query, variables)
	return query.Repository.Ref.ID != "", err
}

func getTags(httpClient *http.Client, repo ghrepo.Interface, limit int) ([]tag, error) {
	client, err := api.NewRESTClient(httpClient, repo.RepoHost())
	if err != nil {
		return nil, err
	}

	u, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "tags")
	if err != nil {
		return nil, err
	}
	u.SetQuery("per_page", strconv.Itoa(limit))

	req, err := client.NewRequest(context.Background(), http.MethodGet, u.String(), nil,
		githubrest.WithHeader("Content-Type", "application/json; charset=utf-8"))
	if err != nil {
		return nil, err
	}

	var tags []tag
	if _, err := client.Do(req, &tags); err != nil {
		return nil, err
	}
	return tags, nil
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

	client, err := api.NewRESTClient(httpClient, repo.RepoHost())
	if err != nil {
		return nil, err
	}

	url, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "releases", "generate-notes")
	if err != nil {
		return nil, err
	}

	req, err := client.NewRequest(context.Background(), http.MethodPost, url.String(), bytes.NewBuffer(bodyBytes),
		githubrest.WithHeader("Accept", "application/vnd.github.v3+json"),
		githubrest.WithHeader("Content-Type", "application/json; charset=utf-8"))
	if err != nil {
		return nil, err
	}

	var rn releaseNotes
	if _, err := client.Do(req, &rn); err != nil {
		// A 404 here means the endpoint is absent rather than the release, which
		// is how an older Enterprise Server reports that it cannot generate
		// notes.
		var errResp *githubrest.ErrorResponse
		if errors.As(err, &errResp) && errResp.StatusCode == http.StatusNotFound {
			return nil, notImplementedError
		}
		return nil, err
	}
	return &rn, nil
}

func publishedReleaseExists(httpClient *http.Client, repo ghrepo.Interface, tagName string) (bool, error) {
	client, err := api.NewRESTClient(httpClient, repo.RepoHost())
	if err != nil {
		return false, err
	}

	url, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "releases", "tags", tagName)
	if err != nil {
		return false, err
	}

	req, err := client.NewRequest(context.Background(), http.MethodHead, url.String(), nil)
	if err != nil {
		return false, err
	}

	// A 404 is an answer here rather than a failure, and Send reports every
	// non-2xx as an error, so it has to be unwrapped rather than read off a
	// status code.
	resp, err := client.Send(req)
	if err != nil {
		var errResp *githubrest.ErrorResponse
		if errors.As(err, &errResp) && errResp.StatusCode == http.StatusNotFound {
			return false, nil
		}
		return false, err
	}
	defer resp.Body.Close()

	return true, nil
}

func createRelease(httpClient *http.Client, repo ghrepo.Interface, params map[string]interface{}) (*shared.Release, error) {
	bodyBytes, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	client, err := api.NewRESTClient(httpClient, repo.RepoHost())
	if err != nil {
		return nil, err
	}

	url, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "releases")
	if err != nil {
		return nil, err
	}

	req, err := client.NewRequest(context.Background(), http.MethodPost, url.String(), bytes.NewBuffer(bodyBytes),
		githubrest.WithHeader("Content-Type", "application/json; charset=utf-8"))
	if err != nil {
		return nil, err
	}

	// Send rather than Do, because the 404 branch below reads response headers
	// to tell a missing repository from a token without the workflow scope.
	resp, err := client.Send(req)
	if err != nil {
		var errResp *githubrest.ErrorResponse
		if errors.As(err, &errResp) && errResp.StatusCode == http.StatusNotFound && !tokenHasWorkflowScope(errResp.Headers) {
			return nil, &errMissingRequiredWorkflowScope{
				Hostname: ghauth.NormalizeHostname(errResp.RequestURL.Hostname()),
			}
		}
		return nil, err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var newRelease shared.Release
	err = json.Unmarshal(b, &newRelease)
	return &newRelease, err
}

func publishRelease(httpClient *http.Client, releaseURL safeurl.SafeURL, discussionCategory string, isLatest *bool) (*shared.Release, error) {
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
	client, err := api.NewRESTClientForURL(httpClient, releaseURL.String())
	if err != nil {
		return nil, err
	}

	req, err := client.NewRequest(context.Background(), http.MethodPatch, releaseURL.String(), bytes.NewBuffer(bodyBytes),
		githubrest.WithHeader("Content-Type", "application/json"))
	if err != nil {
		return nil, err
	}

	var release shared.Release
	if _, err := client.Do(req, &release); err != nil {
		return nil, err
	}
	return &release, nil
}

func deleteRelease(httpClient *http.Client, releaseURL safeurl.SafeURL) error {
	client, err := api.NewRESTClientForURL(httpClient, releaseURL.String())
	if err != nil {
		return err
	}

	req, err := client.NewRequest(context.Background(), http.MethodDelete, releaseURL.String(), nil)
	if err != nil {
		return err
	}

	// A nil receiver discards the body, which is what the drain this replaces
	// was for.
	_, err = client.Do(req, nil)
	return err
}

// tokenHasWorkflowScope checks if the token behind the given response headers
// has the workflow scope.
// Tokens that do not have OAuth scopes are assumed to have the workflow scope.
func tokenHasWorkflowScope(headers http.Header) bool {
	scopes := headers.Get("X-Oauth-Scopes")

	// Return true when no scopes are present - no scopes in this header
	// means that the user is probably authenticating with a token type other
	// than an OAuth token, and we don't know what this token's scopes actually are.
	if scopes == "" {
		return true
	}

	return slices.Contains(strings.Split(scopes, ","), "workflow")
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
