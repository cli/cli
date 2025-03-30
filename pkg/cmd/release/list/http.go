package list

import (
	"net/http"
	"strings"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/shurcooL/githubv4"
)

var releaseFields = []string{
	"name",
	"tagName",
	"isDraft",
	"isLatest",
	"isPrerelease",
	"createdAt",
	"publishedAt",
}

type Release struct {
	Name         string
	TagName      string
	IsDraft      bool
	IsLatest     bool
	IsPrerelease bool
	CreatedAt    time.Time
	PublishedAt  time.Time
}

func (r *Release) ExportData(fields []string) map[string]interface{} {
	return cmdutil.StructExportData(r, fields)
}

func fetchReleases(httpClient *http.Client, repo ghrepo.Interface, limit int, excludeDrafts bool, excludePreReleases bool, order string) ([]Release, error) {
	type responseData struct {
		Repository struct {
			Releases struct {
				Nodes    []Release
				PageInfo struct {
					HasNextPage bool
					EndCursor   string
				}
			} `graphql:"releases(first: $perPage, orderBy: {field: CREATED_AT, direction: $direction}, after: $endCursor)"`
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
		"direction": githubv4.OrderDirection(strings.ToUpper(order)),
	}

	gql := api.NewClientFromHTTP(httpClient)

	var releases []Release
loop:
	for {
		var query responseData
		err := gql.Query(repo.RepoHost(), "RepositoryReleaseList", &query, variables)
		if err != nil {
			return nil, err
		}

		for _, r := range query.Repository.Releases.Nodes {
			if excludeDrafts && r.IsDraft {
				continue
			}
			if excludePreReleases && r.IsPrerelease {
				continue
			}
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
