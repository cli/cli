package list

import (
	"context"
	"net/http"
	"reflect"
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
	Owner        string
	Repositories []Repository
	TotalCount   int
}

func listRepos(client *http.Client, hostname string, limit int, owner string, filter FilterOptions) (*RepositoryList, error) {
	perPage := limit
	if perPage > 100 {
		perPage = 100
	}

	variables := map[string]interface{}{
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

	var ownerConnection string
	if owner == "" {
		ownerConnection = `graphql:"repositoryOwner: viewer"`
	} else {
		ownerConnection = `graphql:"repositoryOwner(login: $owner)"`
		variables["owner"] = githubv4.String(owner)
	}

	type repositoryOwner struct {
		Login        string
		Repositories struct {
			Nodes      []Repository
			TotalCount int
			PageInfo   struct {
				HasNextPage bool
				EndCursor   string
			}
		} `graphql:"repositories(first: $perPage, after: $endCursor, privacy: $privacy, isFork: $fork, ownerAffiliations: OWNER, orderBy: { field: PUSHED_AT, direction: DESC })"`
	}
	query := reflect.StructOf([]reflect.StructField{
		{
			Name: "RepositoryOwner",
			Type: reflect.TypeOf(repositoryOwner{}),
			Tag:  reflect.StructTag(ownerConnection),
		},
	})

	listResult := RepositoryList{}
pagination:
	for {
		result := reflect.New(query)
		gql := graphql.NewClient(ghinstance.GraphQLEndpoint(hostname), client)
		err := gql.QueryNamed(context.Background(), "RepositoryList", result.Interface(), variables)
		if err != nil {
			return nil, err
		}

		owner := result.Elem().FieldByName("RepositoryOwner").Interface().(repositoryOwner)
		listResult.TotalCount = owner.Repositories.TotalCount
		listResult.Owner = owner.Login

		for _, repo := range owner.Repositories.Nodes {
			listResult.Repositories = append(listResult.Repositories, repo)
			if len(listResult.Repositories) >= limit {
				break pagination
			}
		}

		if !owner.Repositories.PageInfo.HasNextPage {
			break
		}
		variables["endCursor"] = githubv4.String(owner.Repositories.PageInfo.EndCursor)
	}

	return &listResult, nil
}
