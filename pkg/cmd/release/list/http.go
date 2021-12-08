package list

import (
	"context"
	"net/http"
	"time"

	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	graphql "github.com/cli/shurcooL-graphql"
	"github.com/shurcooL/githubv4"
)

type Release struct {
	Name         string
	TagName      string
	IsDraft      bool
	IsPrerelease bool
	CreatedAt    time.Time
	PublishedAt  time.Time
}

func fetchReleases(httpClient *http.Client, repo ghrepo.Interface, limit int) ([]Release, error) {
	type responseData struct {
		Repository struct {
			Releases struct {
				Nodes    []Release
				PageInfo struct {
					HasNextPage bool
					EndCursor   string
				}
			} `graphql:"releases(first: $perPage, orderBy: {field: CREATED_AT, direction: DESC}, after: $endCursor)"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}

	perPage := limit
	if limit > 100 {
		perPage = 100
	}

	variables := map[string]interface{}{
		"owner":     githubv4.String(repo.RepoOwner()),
		"name":      githubv4.String(repo.RepoName()),
		"perPage":   githubv4.Int(perPage),
		"endCursor": (*githubv4.String)(nil),
	}

	gql := graphql.NewClient(ghinstance.GraphQLEndpoint(repo.RepoHost()), httpClient)

	var releases []Release
loop:
	for {
		var query responseData
		err := gql.QueryNamed(context.Background(), "RepositoryReleaseList", &query, variables)
		if err != nil {
			return nil, err
		}

		for _, r := range query.Repository.Releases.Nodes {
			releases = append(releases, r)
			if len(releases) == limit {
				break loop
			}
		}

		if !query.Repository.Releases.PageInfo.HasNextPage {
			break
		}
		variables["endCursor"] = githubv4.String(query.Repository.Releases.PageInfo.EndCursor)
	}

	return releases, nil
}
