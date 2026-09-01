package edit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
	"github.com/cli/cli/v2/pkg/cmd/release/shared"
	"github.com/shurcooL/githubv4"
)

func editRelease(httpClient *http.Client, repo ghrepo.Interface, releaseID int64, params map[string]any) (*shared.Release, error) {
	bodyBytes, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	url, err := safeurl.JoinPathWithHostPrefix(ghinstance.RESTPrefix(repo.RepoHost()), "repos", repo.RepoOwner(), repo.RepoName(), "releases", strconv.FormatInt(releaseID, 10))
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("PATCH", url.String(), bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	// TODO(api-client-rollout)
	// This has been deferred from moving to api.Client because its return shape depends on the response status code, which api.Client.REST does not expose on success.
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
