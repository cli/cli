package view

import (
	"context"
	"net/http"
	"time"

	"github.com/cli/cli/internal/ghinstance"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/shurcooL/githubv4"
	"github.com/shurcooL/graphql"
)

type Release struct {
	TagName      string
	Name         string
	Description  string
	URL          string
	IsDraft      bool
	IsPrerelease bool
	PublishedAt  time.Time

	Author struct {
		Login string
	}

	ReleaseAssets struct {
		Nodes []struct {
			Name string
			Size int
		}
	} `graphql:"releaseAssets(first: 100)"`
}

func fetchRelease(httpClient *http.Client, repo ghrepo.Interface, tagName string) (*Release, error) {
	var query struct {
		Repository struct {
			Release *Release `graphql:"release(tagName: $tagName)"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}

	variables := map[string]interface{}{
		"owner":   githubv4.String(repo.RepoOwner()),
		"name":    githubv4.String(repo.RepoName()),
		"tagName": githubv4.String(tagName),
	}

	gql := graphql.NewClient(ghinstance.GraphQLEndpoint(repo.RepoHost()), httpClient)
	err := gql.QueryNamed(context.Background(), "RepositoryReleaseByTag", &query, variables)
	return query.Repository.Release, err
}
