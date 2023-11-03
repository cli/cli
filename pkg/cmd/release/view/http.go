package view

import (
	"context"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/shurcooL/githubv4"
)

func FetchRelease(ctx context.Context, httpClient *http.Client, repo ghrepo.Interface, tagName string) (*api.Release, error) {
	variables := map[string]interface{}{
		"owner":   githubv4.String(repo.RepoOwner()),
		"name":    githubv4.String(repo.RepoName()),
		"tagName": githubv4.String(tagName),
	}

	var query struct {
		Repository struct {
			Release *api.Release `graphql:"release(tagName: $tagName)"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}

	gql := api.NewClientFromHTTP(httpClient)
	if err := gql.QueryWithContext(ctx, repo.RepoHost(), "RepositoryReleaseByTag", &query, variables); err != nil {
		return nil, err
	}

	return query.Repository.Release, nil
}

func FetchLatestRelease(ctx context.Context, httpClient *http.Client, repo ghrepo.Interface) (*api.Release, error) {
	var listResponse struct {
		Repository struct {
			Releases struct {
				Nodes []struct {
					TagName  string
					IsLatest bool
				}
				PageInfo struct {
					HasNextPage bool
					EndCursor   string
				}
			} `graphql:"releases(first: $perPage, orderBy: {field: CREATED_AT, direction: DESC}, after: $endCursor)"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}

	variables := map[string]interface{}{
		"owner":     githubv4.String(repo.RepoOwner()),
		"name":      githubv4.String(repo.RepoName()),
		"perPage":   githubv4.Int(100),
		"endCursor": (*githubv4.String)(nil),
	}

	gql := api.NewClientFromHTTP(httpClient)

	var tagName string
loop:
	for {
		if err := gql.QueryWithContext(ctx, repo.RepoHost(), "RepositoryReleases", &listResponse, variables); err != nil {
			return nil, err
		}

		for _, r := range listResponse.Repository.Releases.Nodes {
			if r.IsLatest {
				tagName = r.TagName
				break loop
			}
		}

		if !listResponse.Repository.Releases.PageInfo.HasNextPage {
			break
		}
		variables["endCursor"] = githubv4.String(listResponse.Repository.Releases.PageInfo.EndCursor)
	}

	return FetchRelease(ctx, httpClient, repo, tagName)
}
