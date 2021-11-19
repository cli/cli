package api

import (
	"context"
	"time"

	graphql "github.com/cli/shurcooL-graphql"
	"github.com/shurcooL/githubv4"
)

type Comments struct {
	Nodes      []Comment
	TotalCount int
	PageInfo   struct {
		HasNextPage bool
		EndCursor   string
	}
}

type Comment struct {
	Author              Author         `json:"author"`
	AuthorAssociation   string         `json:"authorAssociation"`
	Body                string         `json:"body"`
	CreatedAt           time.Time      `json:"createdAt"`
	IncludesCreatedEdit bool           `json:"includesCreatedEdit"`
	IsMinimized         bool           `json:"isMinimized"`
	MinimizedReason     string         `json:"minimizedReason"`
	ReactionGroups      ReactionGroups `json:"reactionGroups"`
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
