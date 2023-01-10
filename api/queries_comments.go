package api

import (
	"time"

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

func (cs Comments) CurrentUserComments() []Comment {
	var comments []Comment
	for _, c := range cs.Nodes {
		if c.ViewerDidAuthor {
			comments = append(comments, c)
		}
	}
	return comments
}

type Comment struct {
	ID                  string         `json:"id"`
	Author              CommentAuthor  `json:"author"`
	AuthorAssociation   string         `json:"authorAssociation"`
	Body                string         `json:"body"`
	CreatedAt           time.Time      `json:"createdAt"`
	IncludesCreatedEdit bool           `json:"includesCreatedEdit"`
	IsMinimized         bool           `json:"isMinimized"`
	MinimizedReason     string         `json:"minimizedReason"`
	ReactionGroups      ReactionGroups `json:"reactionGroups"`
	URL                 string         `json:"url,omitempty"`
	ViewerDidAuthor     bool           `json:"viewerDidAuthor"`
}

type CommentCreateInput struct {
	Body      string
	SubjectId string
}

type CommentUpdateInput struct {
	Body      string
	CommentId string
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
			SubjectID: githubv4.ID(params.SubjectId),
		},
	}

	err := client.Mutate(repoHost, "CommentCreate", &mutation, variables)
	if err != nil {
		return "", err
	}

	return mutation.AddComment.CommentEdge.Node.URL, nil
}

func CommentUpdate(client *Client, repoHost string, params CommentUpdateInput) (string, error) {
	var mutation struct {
		UpdateIssueComment struct {
			IssueComment struct {
				URL string
			}
		} `graphql:"updateIssueComment(input: $input)"`
	}

	variables := map[string]interface{}{
		"input": githubv4.UpdateIssueCommentInput{
			Body: githubv4.String(params.Body),
			ID:   githubv4.ID(params.CommentId),
		},
	}

	err := client.Mutate(repoHost, "CommentUpdate", &mutation, variables)
	if err != nil {
		return "", err
	}

	return mutation.UpdateIssueComment.IssueComment.URL, nil
}

func (c Comment) Identifier() string {
	return c.ID
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
	return c.URL
}

func (c Comment) Reactions() ReactionGroups {
	return c.ReactionGroups
}

func (c Comment) Status() string {
	return ""
}
