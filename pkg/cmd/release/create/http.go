package create

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/release/shared"
	"github.com/shurcooL/githubv4"
	"github.com/shurcooL/graphql"
)

type Ref struct {
	Name   string `graphql:"name"`
	Prefix string `graphql:"refPrefix"`
}

const maxItemsPerPage = 100

func fetchTags(httpClient *http.Client, repo ghrepo.Interface, limit int) ([]Ref, error) {
	type responseData struct {
		Repository struct {
			Refs struct {
				Nodes    []Ref
				PageInfo struct {
					HasNextPage bool
					EndCursor   string
				}
			} `graphql:"refs(first: $perPage, orderBy: {field: TAG_COMMIT_DATE, direction: DESC}, after: $endCursor, refPrefix: $prefix)"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}

	perPage := limit
	if limit > maxItemsPerPage {
		perPage = maxItemsPerPage
	}

	variables := map[string]interface{}{
		"owner":     githubv4.String(repo.RepoOwner()),
		"name":      githubv4.String(repo.RepoName()),
		"perPage":   githubv4.Int(perPage),
		"endCursor": (*githubv4.String)(nil),
		"prefix":    githubv4.String("refs/tags/"),
	}

	gql := graphql.NewClient(ghinstance.GraphQLEndpoint(repo.RepoHost()), httpClient)

	var tags []Ref
loop:
	for {
		var query responseData
		err := gql.QueryNamed(context.Background(), "RepositoryReleaseList", &query, variables)
		if err != nil {
			return nil, err
		}

		for _, tag := range query.Repository.Refs.Nodes {
			tags = append(tags, tag)
			if len(tags) == limit {
				break loop
			}
		}

		if !query.Repository.Refs.PageInfo.HasNextPage {
			break
		}

		variables["endCursor"] = githubv4.String(query.Repository.Refs.PageInfo.EndCursor)
	}

	return tags, nil
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

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var newRelease shared.Release
	err = json.Unmarshal(b, &newRelease)
	return &newRelease, err
}

func publishRelease(httpClient *http.Client, releaseURL string) (*shared.Release, error) {
	req, err := http.NewRequest("PATCH", releaseURL, bytes.NewBufferString(`{"draft":false}`))
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

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var release shared.Release
	err = json.Unmarshal(b, &release)
	return &release, err
}
