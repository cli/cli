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

func ReviewsForPullRequest(client *Client, repo ghrepo.Interface, pr *PullRequest) (*PullRequestReviews, error) {
	type response struct {
		Repository struct {
			PullRequest struct {
				Reviews PullRequestReviews `graphql:"reviews(first: 100, after: $endCursor)"`
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

	var reviews []PullRequestReview
	for {
		var query response
		err := gql.QueryNamed(context.Background(), "ReviewsForPullRequest", &query, variables)
		if err != nil {
			return nil, err
		}

		reviews = append(reviews, query.Repository.PullRequest.Reviews.Nodes...)
		if !query.Repository.PullRequest.Reviews.PageInfo.HasNextPage {
			break
		}
		variables["endCursor"] = githubv4.String(query.Repository.PullRequest.Reviews.PageInfo.EndCursor)
	}

	return &PullRequestReviews{Nodes: reviews, TotalCount: len(reviews)}, nil
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
