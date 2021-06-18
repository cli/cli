package view

import (
	"context"
	"net/http"

	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghinstance"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/shurcooL/githubv4"
	"github.com/shurcooL/graphql"
)

func preloadIssueComments(client *http.Client, repo ghrepo.Interface, issue *api.Issue) error {
	type response struct {
		Node struct {
			Issue struct {
				Comments api.Comments `graphql:"comments(first: 100, after: $endCursor)"`
			} `graphql:"...on Issue"`
		} `graphql:"node(id: $id)"`
	}

	variables := map[string]interface{}{
		"id":        githubv4.ID(issue.ID),
		"endCursor": (*githubv4.String)(nil),
	}
	if issue.Comments.PageInfo.HasNextPage {
		variables["endCursor"] = githubv4.String(issue.Comments.PageInfo.EndCursor)
	} else {
		issue.Comments.Nodes = issue.Comments.Nodes[0:0]
	}

	gql := graphql.NewClient(ghinstance.GraphQLEndpoint(repo.RepoHost()), client)
	for {
		var query response
		err := gql.QueryNamed(context.Background(), "CommentsForIssue", &query, variables)
		if err != nil {
			return err
		}

		issue.Comments.Nodes = append(issue.Comments.Nodes, query.Node.Issue.Comments.Nodes...)
		if !query.Node.Issue.Comments.PageInfo.HasNextPage {
			break
		}
		variables["endCursor"] = githubv4.String(query.Node.Issue.Comments.PageInfo.EndCursor)
	}

	issue.Comments.PageInfo.HasNextPage = false
	return nil
}
