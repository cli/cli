package edit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/cli/cli/v2/internal/githubrest"
	"net/http"
	"strconv"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
	"github.com/cli/cli/v2/pkg/cmd/release/shared"
	"github.com/shurcooL/githubv4"
)

func editRelease(httpClient *http.Client, repo ghrepo.Interface, releaseID int64, params map[string]interface{}) (*shared.Release, error) {
	bodyBytes, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	client, err := api.NewRESTClient(httpClient, repo.RepoHost())
	if err != nil {
		return nil, err
	}

	url, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "releases", strconv.FormatInt(releaseID, 10))
	if err != nil {
		return nil, err
	}

	req, err := client.NewRequest(context.Background(), http.MethodPatch, url.String(), bytes.NewBuffer(bodyBytes),
		githubrest.WithHeader("Content-Type", "application/json; charset=utf-8"))
	if err != nil {
		return nil, err
	}

	var newRelease shared.Release
	if _, err := client.Do(req, &newRelease); err != nil {
		return nil, err
	}
	return &newRelease, nil
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
