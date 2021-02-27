package list

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/cli/cli/internal/ghinstance"
	"github.com/shurcooL/githubv4"
	"github.com/shurcooL/graphql"
)

type Repository struct {
	NameWithOwner string
	Description   string
	IsFork        bool
	IsPrivate     bool
	IsArchived    bool
	PushedAt      time.Time
}

func (r Repository) Info() string {
	var tags []string

	if r.IsPrivate {
		tags = append(tags, "private")
	} else {
		tags = append(tags, "public")
	}
	if r.IsFork {
		tags = append(tags, "fork")
	}
	if r.IsArchived {
		tags = append(tags, "archived")
	}

	return strings.Join(tags, ", ")
}

type RepositoryList struct {
	Repositories []Repository
	TotalCount   int
}

func listRepos(client *http.Client, hostname string, limit int, owner string, filter FilterOptions) (*RepositoryList, error) {
	type query struct {
		RepositoryOwner struct {
			Repositories struct {
				Nodes      []Repository
				TotalCount int
				PageInfo   struct {
					HasNextPage bool
					EndCursor   string
				}
			} `graphql:"repositories(first: $perPage, after: $endCursor, privacy: $privacy, isFork: $fork, ownerAffiliations: OWNER, orderBy: { field: PUSHED_AT, direction: DESC })"`
		} `graphql:"repositoryOwner(login: $owner)"`
	}

	perPage := limit
	if perPage > 100 {
		perPage = 100
	}

	variables := map[string]interface{}{
		"owner":     githubv4.String(owner),
		"perPage":   githubv4.Int(perPage),
		"endCursor": (*githubv4.String)(nil),
	}

	if filter.Visibility != "" {
		variables["privacy"] = githubv4.RepositoryPrivacy(strings.ToUpper(filter.Visibility))
	} else {
		variables["privacy"] = (*githubv4.RepositoryPrivacy)(nil)
	}

	if filter.Fork {
		variables["fork"] = githubv4.Boolean(true)
	} else if filter.Source {
		variables["fork"] = githubv4.Boolean(false)
	} else {
		variables["fork"] = (*githubv4.Boolean)(nil)
	}

	listResult := RepositoryList{}
pagination:
	for {
		var result query
		gql := graphql.NewClient(ghinstance.GraphQLEndpoint(hostname), client)
		err := gql.QueryNamed(context.Background(), "RepositoryList", &result, variables)
		if err != nil {
			return nil, err
		}

		listResult.TotalCount = result.RepositoryOwner.Repositories.TotalCount
		for _, repo := range result.RepositoryOwner.Repositories.Nodes {
			listResult.Repositories = append(listResult.Repositories, repo)
			if len(listResult.Repositories) >= limit {
				break pagination
			}
		}

		if !result.RepositoryOwner.Repositories.PageInfo.HasNextPage {
			break
		}
		variables["endCursor"] = githubv4.String(result.RepositoryOwner.Repositories.PageInfo.EndCursor)
	}

	return &listResult, nil
}
