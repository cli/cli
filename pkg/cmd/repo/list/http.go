package list

import (
	"context"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/cli/cli/internal/ghinstance"
	"github.com/cli/cli/pkg/githubsearch"
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
	FromSearch   bool
}

type FilterOptions struct {
	Visibility  string // private, public
	Fork        bool
	Source      bool
	Language    string
	Archived    bool
	NonArchived bool
}

func listRepos(client *http.Client, hostname string, limit int, owner string, filter FilterOptions) (*RepositoryList, error) {
	if filter.Language != "" || filter.Archived || filter.NonArchived {
		return searchRepos(client, hostname, limit, owner, filter)
	}

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

	gql := graphql.NewClient(ghinstance.GraphQLEndpoint(hostname), client)
	listResult := RepositoryList{}
pagination:
	for {
		result := reflect.New(query)
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

func searchRepos(client *http.Client, hostname string, limit int, owner string, filter FilterOptions) (*RepositoryList, error) {
	type query struct {
		Search struct {
			RepositoryCount int
			Nodes           []struct {
				Repository Repository `graphql:"...on Repository"`
			}
			PageInfo struct {
				HasNextPage bool
				EndCursor   string
			}
		} `graphql:"search(type: REPOSITORY, query: $query, first: $perPage, after: $endCursor)"`
	}

	perPage := limit
	if perPage > 100 {
		perPage = 100
	}

	variables := map[string]interface{}{
		"query":     githubv4.String(searchQuery(owner, filter)),
		"perPage":   githubv4.Int(perPage),
		"endCursor": (*githubv4.String)(nil),
	}

	gql := graphql.NewClient(ghinstance.GraphQLEndpoint(hostname), client)
	listResult := RepositoryList{FromSearch: true}
pagination:
	for {
		var result query
		err := gql.QueryNamed(context.Background(), "RepositoryListSearch", &result, variables)
		if err != nil {
			return nil, err
		}

		listResult.TotalCount = result.Search.RepositoryCount
		for _, node := range result.Search.Nodes {
			if listResult.Owner == "" {
				idx := strings.IndexRune(node.Repository.NameWithOwner, '/')
				listResult.Owner = node.Repository.NameWithOwner[:idx]
			}
			listResult.Repositories = append(listResult.Repositories, node.Repository)
			if len(listResult.Repositories) >= limit {
				break pagination
			}
		}

		if !result.Search.PageInfo.HasNextPage {
			break
		}
		variables["endCursor"] = githubv4.String(result.Search.PageInfo.EndCursor)
	}

	return &listResult, nil
}

func searchQuery(owner string, filter FilterOptions) string {
	q := githubsearch.NewQuery()
	q.SortBy(githubsearch.UpdatedAt, githubsearch.Desc)

	if owner == "" {
		q.OwnedBy("@me")
	} else {
		q.OwnedBy(owner)
	}

	if filter.Fork {
		q.OnlyForks()
	} else {
		q.IncludeForks(!filter.Source)
	}

	if filter.Language != "" {
		q.SetLanguage(filter.Language)
	}

	switch filter.Visibility {
	case "public":
		q.SetVisibility(githubsearch.Public)
	case "private":
		q.SetVisibility(githubsearch.Private)
	}

	if filter.Archived {
		q.SetArchived(true)
	} else if filter.NonArchived {
		q.SetArchived(false)
	}

	return q.String()
}
