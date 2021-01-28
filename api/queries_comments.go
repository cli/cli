package api

import (
	"context"
	"time"

	"github.com/cli/cli/internal/ghrepo"
	"github.com/shurcooL/githubv4"
	"github.com/shurcooL/graphql"
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
	IsMinimized         bool
	MinimizedReason     string
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

type CommentCreateInput struct {
	Body      string
	SubjectId string
}

func CommentCreate(client *Client, repoHost string, params CommentCreateInput) (string, error) {
	var mutation struct {
		AddComment struct {
			CommentEdge struct {
				Node struct {
					URL string
				}
			}
		} `graphql:"addComment(input: $input)"`
	}

	variables := map[string]interface{}{
		"input": githubv4.AddCommentInput{
			Body:      githubv4.String(params.Body),
			SubjectID: graphql.ID(params.SubjectId),
		},
	}

	gql := graphQLClient(client.http, repoHost)
	err := gql.MutateNamed(context.Background(), "CommentCreate", &mutation, variables)
	if err != nil {
		return "", err
	}

	return mutation.AddComment.CommentEdge.Node.URL, nil
}

func commentsFragment() string {
	return `comments(last: 1) {
						nodes {
							author {
								login
							}
							authorAssociation
							body
							createdAt
							includesCreatedEdit
							isMinimized
							minimizedReason
							` + reactionGroupsFragment() + `
						}
						totalCount
					}`
}

func (c Comment) AuthorLogin() string {
	return c.Author.Login
}

func (c Comment) Association() string {
	return c.AuthorAssociation
}

func (c Comment) Content() string {
	return c.Body
}

func (c Comment) Created() time.Time {
	return c.CreatedAt
}

func (c Comment) HiddenReason() string {
	return c.MinimizedReason
}

func (c Comment) IsEdited() bool {
	return c.IncludesCreatedEdit
}

func (c Comment) IsHidden() bool {
	return c.IsMinimized
}

func (c Comment) Link() string {
	return ""
}

func (c Comment) Reactions() ReactionGroups {
	return c.ReactionGroups
}

func (c Comment) Status() string {
	return ""
}
