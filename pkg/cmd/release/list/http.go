package list

import (
	"net/http"
	"strings"
	"time"

	"github.com/cli/cli/v2/api"
	fd "github.com/cli/cli/v2/internal/featuredetection"
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
	"isImmutable",
	"createdAt",
	"publishedAt",
}

type Release struct {
	Name         string
	TagName      string
	IsDraft      bool
	IsImmutable  bool `graphql:"immutable"`
	IsLatest     bool
	IsPrerelease bool
	CreatedAt    time.Time
	PublishedAt  time.Time
}

func (r *Release) ExportData(fields []string) map[string]interface{} {
	return cmdutil.StructExportData(r, fields)
}

func fetchReleases(httpClient *http.Client, repo ghrepo.Interface, limit int, excludeDrafts bool, excludePreReleases bool, order string, releaseFeatures fd.ReleaseFeatures) ([]Release, error) {
	// TODO: immutableReleaseFullSupport
	// This is a temporary workaround until all supported GHES versions fully
	// support immutable releases, which would probably be when GHES 3.18 goes
	// EOL. At that point we can remove this if statement.
	//
	// Note 1: This could have been done differently by using two separate query
	// types or even using plain text/string queries. But, both would require us
	// to refactor them back in the future, to the single, strongly-typed query
	// approach as it was before. So, duplicating the entire function for now
	// seems like the lesser evil, with a quicker and less risky clean up in the
	// near future.
	//
	// Note 2: We couldn't use GraphQL directives like `@include(condition)` or
	// `@skip(condition)` here because if the field doesn't exist on the schema
	// then the whole query would still fail regardless of the condition being
	// met or not.
	if !releaseFeatures.ImmutableReleases {
		return fetchReleasesWithoutImmutableReleases(httpClient, repo, limit, excludeDrafts, excludePreReleases, order)
	}

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

// TODO: immutableReleaseFullSupport
// This is a temporary workaround until all supported GHES versions fully
// support immutable releases, which would be when GHES 3.18 goes EOL. At that
// point we can remove this function.
func fetchReleasesWithoutImmutableReleases(httpClient *http.Client, repo ghrepo.Interface, limit int, excludeDrafts bool, excludePreReleases bool, order string) ([]Release, error) {
	type releaseOld struct {
		Name         string
		TagName      string
		IsDraft      bool
		IsLatest     bool
		IsPrerelease bool
		CreatedAt    time.Time
		PublishedAt  time.Time
	}

	fromReleaseOld := func(old releaseOld) Release {
		return Release{
			Name:         old.Name,
			TagName:      old.TagName,
			IsDraft:      old.IsDraft,
			IsLatest:     old.IsLatest,
			IsPrerelease: old.IsPrerelease,
			CreatedAt:    old.CreatedAt,
			PublishedAt:  old.PublishedAt,
		}
	}

	type responseData struct {
		Repository struct {
			Releases struct {
				Nodes    []releaseOld
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
			releases = append(releases, fromReleaseOld(r))
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
