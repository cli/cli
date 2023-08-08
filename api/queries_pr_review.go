package api

import (
	"errors"
	"time"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/shurcooL/githubv4"
)

type PullRequestReviewState int

const (
	ReviewApprove PullRequestReviewState = iota
	ReviewRequestChanges
	ReviewComment
)

type PullRequestReviewInput struct {
	Body  string
	State PullRequestReviewState
}

type PullRequestReviewThreadInput struct {
	Path      string
	Body      string
	Line      int
	StartLine *int
}

type PullRequestReviews struct {
	Nodes    []PullRequestReview
	PageInfo struct {
		HasNextPage bool
		EndCursor   string
	}
	TotalCount int
}

type PullRequestReview struct {
	ID                  string         `json:"id"`
	Author              CommentAuthor  `json:"author"`
	AuthorAssociation   string         `json:"authorAssociation"`
	Body                string         `json:"body"`
	SubmittedAt         *time.Time     `json:"submittedAt"`
	IncludesCreatedEdit bool           `json:"includesCreatedEdit"`
	ReactionGroups      ReactionGroups `json:"reactionGroups"`
	State               string         `json:"state"`
	URL                 string         `json:"url,omitempty"`
	Commit              Commit         `json:"commit"`
}

func AddReview(client *Client, repo ghrepo.Interface, pr *PullRequest, input *PullRequestReviewInput) error {
	var mutation struct {
		AddPullRequestReview struct {
			ClientMutationID string
		} `graphql:"addPullRequestReview(input:$input)"`
	}

	state := githubv4.PullRequestReviewEventComment
	switch input.State {
	case ReviewApprove:
		state = githubv4.PullRequestReviewEventApprove
	case ReviewRequestChanges:
		state = githubv4.PullRequestReviewEventRequestChanges
	}

	body := githubv4.String(input.Body)
	variables := map[string]interface{}{
		"input": githubv4.AddPullRequestReviewInput{
			PullRequestID: pr.ID,
			Event:         &state,
			Body:          &body,
		},
	}

	return client.Mutate(repo.RepoHost(), "PullRequestReviewAdd", &mutation, variables)
}

func AddReviewThread(client *Client, repo ghrepo.Interface, pr *PullRequest, input *PullRequestReviewThreadInput) error {
	var mutation struct {
		AddPullRequestReviewThread struct {
			Thread struct {
				ID string
			}
		} `graphql:"addPullRequestReviewThread(input:$input)"`
	}

	id := githubv4.ID(pr.ID)
	path := githubv4.String(input.Path)
	body := githubv4.String(input.Body)
	line := githubv4.Int(input.Line)
	var startLine *githubv4.Int
	if input.StartLine == nil {
		startLine = nil
	} else {
		startLineInt := githubv4.Int(*input.StartLine)
		startLine = &startLineInt
	}

	variables := map[string]interface{}{
		"input": githubv4.AddPullRequestReviewThreadInput{
			PullRequestID: &id,
			Path:          path,
			Body:          body,
			Line:          &line,
			StartLine:     startLine,
		},
	}

	err := client.Mutate(repo.RepoHost(), "PullRequestReviewAddThread", &mutation, variables)
	if err != nil {
		return err
	}

	if mutation.AddPullRequestReviewThread.Thread.ID == "" {
		return errors.New("failed to create review thread")
	}

	return nil
}

func (prr PullRequestReview) Identifier() string {
	return prr.ID
}

func (prr PullRequestReview) AuthorLogin() string {
	return prr.Author.Login
}

func (prr PullRequestReview) Association() string {
	return prr.AuthorAssociation
}

func (prr PullRequestReview) Content() string {
	return prr.Body
}

func (prr PullRequestReview) Created() time.Time {
	if prr.SubmittedAt == nil {
		return time.Time{}
	}
	return *prr.SubmittedAt
}

func (prr PullRequestReview) HiddenReason() string {
	return ""
}

func (prr PullRequestReview) IsEdited() bool {
	return prr.IncludesCreatedEdit
}

func (prr PullRequestReview) IsHidden() bool {
	return false
}

func (prr PullRequestReview) Link() string {
	return prr.URL
}

func (prr PullRequestReview) Reactions() ReactionGroups {
	return prr.ReactionGroups
}

func (prr PullRequestReview) Status() string {
	return prr.State
}
