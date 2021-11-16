package list

import (
	"time"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/go-gh/pkg/api"
	graphql "github.com/samcoe/go-graphql-client"
)

type Release struct {
	Name         string
	TagName      string
	IsDraft      bool
	IsPrerelease bool
	CreatedAt    time.Time
	PublishedAt  time.Time
}

func fetchReleases(gqlClient api.GQLClient, repo ghrepo.Interface, limit int) ([]Release, error) {
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
		"owner":     graphql.String(repo.RepoOwner()),
		"name":      graphql.String(repo.RepoName()),
		"perPage":   graphql.Int(perPage),
		"endCursor": (*graphql.String)(nil),
	}

	var releases []Release
loop:
	for {
		var query responseData
		err := gqlClient.Query("RepositoryReleaseList", &query, variables)
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
		variables["endCursor"] = graphql.String(query.Repository.Releases.PageInfo.EndCursor)
	}

	return releases, nil
}
