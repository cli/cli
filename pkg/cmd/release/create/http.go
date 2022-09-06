package create

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/release/shared"
	"github.com/shurcooL/githubv4"
)

type tag struct {
	Name string `json:"name"`
}

type releaseNotes struct {
	Name string `json:"name"`
	Body string `json:"body"`
}

var notImplementedError = errors.New("not implemented")

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

func publishRelease(httpClient *http.Client, releaseURL string, discussionCategory string) (*shared.Release, error) {
	params := map[string]interface{}{"draft": false}
	if discussionCategory != "" {
		params["discussion_category_name"] = discussionCategory
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
