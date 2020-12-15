package api

import (
	"context"
	"time"

	"github.com/cli/cli/internal/ghrepo"
	"github.com/shurcooL/githubv4"
)

type Comments struct {
	Nodes      []Comment
	TotalCount int
	PageInfo   PageInfo
}

type Comment struct {
	Author              Author
	AuthorAssociation   string
	Body                string
	CreatedAt           time.Time
	IncludesCreatedEdit bool
	ReactionGroups      ReactionGroups
}

type PageInfo struct {
	HasNextPage bool
	EndCursor   string
}

func CommentsForIssue(client *Client, repo ghrepo.Interface, issue *Issue) (*Comments, error) {
	type response struct {
		Repository struct {
			Issue struct {
				Comments Comments `graphql:"comments(first: 100, after: $endCursor)"`
			} `graphql:"issue(number: $number)"`
		} `graphql:"repository(owner: $owner, name: $repo)"`
	}

	variables := map[string]interface{}{
		"owner":     githubv4.String(repo.RepoOwner()),
		"repo":      githubv4.String(repo.RepoName()),
		"number":    githubv4.Int(issue.Number),
		"endCursor": (*githubv4.String)(nil),
	}

	gql := graphQLClient(client.http, repo.RepoHost())

	var comments []Comment
	for {
		var query response
		err := gql.QueryNamed(context.Background(), "CommentsForIssue", &query, variables)
		if err != nil {
			return nil, err
		}

		comments = append(comments, query.Repository.Issue.Comments.Nodes...)
		if !query.Repository.Issue.Comments.PageInfo.HasNextPage {
			break
		}
		variables["endCursor"] = githubv4.String(query.Repository.Issue.Comments.PageInfo.EndCursor)
	}

	return &Comments{Nodes: comments, TotalCount: len(comments)}, nil
}

func CommentsForPullRequest(client *Client, repo ghrepo.Interface, pr *PullRequest) (*Comments, error) {
	type response struct {
		Repository struct {
			PullRequest struct {
				Comments Comments `graphql:"comments(first: 100, after: $endCursor)"`
			} `graphql:"pullRequest(number: $number)"`
		} `graphql:"repository(owner: $owner, name: $repo)"`
	}

	variables := map[string]interface{}{
		"owner":     githubv4.String(repo.RepoOwner()),
		"repo":      githubv4.String(repo.RepoName()),
		"number":    githubv4.Int(pr.Number),
		"endCursor": (*githubv4.String)(nil),
	}

	gql := graphQLClient(client.http, repo.RepoHost())

	var comments []Comment
	for {
		var query response
		err := gql.QueryNamed(context.Background(), "CommentsForPullRequest", &query, variables)
		if err != nil {
			return nil, err
		}

		comments = append(comments, query.Repository.PullRequest.Comments.Nodes...)
		if !query.Repository.PullRequest.Comments.PageInfo.HasNextPage {
			break
		}
		variables["endCursor"] = githubv4.String(query.Repository.PullRequest.Comments.PageInfo.EndCursor)
	}

	return &Comments{Nodes: comments, TotalCount: len(comments)}, nil
}
