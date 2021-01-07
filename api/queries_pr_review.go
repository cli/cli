package api

import (
	"context"
	"time"

	"github.com/cli/cli/internal/ghrepo"
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

type PullRequestReviews struct {
	Nodes      []PullRequestReview
	PageInfo   PageInfo
	TotalCount int
}

type PullRequestReview struct {
	Author              Author
	AuthorAssociation   string
	Body                string
	CreatedAt           time.Time
	IncludesCreatedEdit bool
	ReactionGroups      ReactionGroups
	State               string
	URL                 string
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

	gql := graphQLClient(client.http, repo.RepoHost())
	return gql.MutateNamed(context.Background(), "PullRequestReviewAdd", &mutation, variables)
}

func pullRequestReviewsFragment() string {
	return `reviews(last: 100) {
						nodes {
							author {
							  login
							}
							authorAssociation
							body
							createdAt
							includesCreatedEdit
							state
							url
							` + reactionGroupsFragment() + `
						}
						totalCount
					}`
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
	return prr.CreatedAt
}

func (prr PullRequestReview) IsEdited() bool {
	return prr.IncludesCreatedEdit
}

func (prr PullRequestReview) Reactions() ReactionGroups {
	return prr.ReactionGroups
}

func (prr PullRequestReview) Status() string {
	return prr.State
}

func (prr PullRequestReview) Link() string {
	return prr.URL
}
