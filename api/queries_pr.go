package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/shurcooL/githubv4"
)

type PullRequestAndTotalCount struct {
	TotalCount   int
	PullRequests []PullRequest
	SearchCapped bool
}

type PullRequest struct {
	ID               string
	Number           int
	Title            string
	State            string
	Closed           bool
	URL              string
	BaseRefName      string
	HeadRefName      string
	Body             string
	Mergeable        string
	Additions        int
	Deletions        int
	ChangedFiles     int
	MergeStateStatus string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	ClosedAt         *time.Time
	MergedAt         *time.Time

	MergeCommit          *Commit
	PotentialMergeCommit *Commit

	Files struct {
		Nodes []PullRequestFile
	}

	Author              Author
	MergedBy            *Author
	HeadRepositoryOwner Owner
	HeadRepository      *PRRepository
	IsCrossRepository   bool
	IsDraft             bool
	MaintainerCanModify bool

	BaseRef struct {
		BranchProtectionRule struct {
			RequiresStrictStatusChecks   bool
			RequiredApprovingReviewCount int
		}
	}

	ReviewDecision string

	Commits struct {
		TotalCount int
		Nodes      []PullRequestCommit
	}
	StatusCheckRollup struct {
		Nodes []struct {
			Commit struct {
				StatusCheckRollup struct {
					Contexts struct {
						Nodes    []CheckContext
						PageInfo struct {
							HasNextPage bool
							EndCursor   string
						}
					}
				}
			}
		}
	}

	Assignees      Assignees
	Labels         Labels
	ProjectCards   ProjectCards
	Milestone      *Milestone
	Comments       Comments
	ReactionGroups ReactionGroups
	Reviews        PullRequestReviews
	LatestReviews  PullRequestReviews
	ReviewRequests ReviewRequests
}

type CheckContext struct {
	TypeName    string    `json:"__typename"`
	Name        string    `json:"name"`
	Context     string    `json:"context,omitempty"`
	State       string    `json:"state,omitempty"`
	Status      string    `json:"status"`
	Conclusion  string    `json:"conclusion"`
	StartedAt   time.Time `json:"startedAt"`
	CompletedAt time.Time `json:"completedAt"`
	DetailsURL  string    `json:"detailsUrl"`
	TargetURL   string    `json:"targetUrl,omitempty"`
}

type PRRepository struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Commit loads just the commit SHA and nothing else
type Commit struct {
	OID string `json:"oid"`
}

type PullRequestCommit struct {
	Commit PullRequestCommitCommit
}

// PullRequestCommitCommit contains full information about a commit
type PullRequestCommitCommit struct {
	OID     string `json:"oid"`
	Authors struct {
		Nodes []struct {
			Name  string
			Email string
			User  GitHubUser
		}
	}
	MessageHeadline string
	MessageBody     string
	CommittedDate   time.Time
	AuthoredDate    time.Time
}

type PullRequestFile struct {
	Path      string `json:"path"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
}

type ReviewRequests struct {
	Nodes []struct {
		RequestedReviewer RequestedReviewer
	}
}

type RequestedReviewer struct {
	TypeName     string `json:"__typename"`
	Login        string `json:"login"`
	Name         string `json:"name"`
	Slug         string `json:"slug"`
	Organization struct {
		Login string `json:"login"`
	} `json:"organization"`
}

func (r RequestedReviewer) LoginOrSlug() string {
	if r.TypeName == teamTypeName {
		return fmt.Sprintf("%s/%s", r.Organization.Login, r.Slug)
	}
	return r.Login
}

const teamTypeName = "Team"

func (r ReviewRequests) Logins() []string {
	logins := make([]string, len(r.Nodes))
	for i, r := range r.Nodes {
		logins[i] = r.RequestedReviewer.LoginOrSlug()
	}
	return logins
}

func (pr PullRequest) HeadLabel() string {
	if pr.IsCrossRepository {
		return fmt.Sprintf("%s:%s", pr.HeadRepositoryOwner.Login, pr.HeadRefName)
	}
	return pr.HeadRefName
}

func (pr PullRequest) Link() string {
	return pr.URL
}

func (pr PullRequest) Identifier() string {
	return pr.ID
}

func (pr PullRequest) IsOpen() bool {
	return pr.State == "OPEN"
}

type PullRequestReviewStatus struct {
	ChangesRequested bool
	Approved         bool
	ReviewRequired   bool
}

func (pr *PullRequest) ReviewStatus() PullRequestReviewStatus {
	var status PullRequestReviewStatus
	switch pr.ReviewDecision {
	case "CHANGES_REQUESTED":
		status.ChangesRequested = true
	case "APPROVED":
		status.Approved = true
	case "REVIEW_REQUIRED":
		status.ReviewRequired = true
	}
	return status
}

type PullRequestChecksStatus struct {
	Pending int
	Failing int
	Passing int
	Total   int
}

func (pr *PullRequest) ChecksStatus() (summary PullRequestChecksStatus) {
	if len(pr.StatusCheckRollup.Nodes) == 0 {
		return
	}
	commit := pr.StatusCheckRollup.Nodes[0].Commit
	for _, c := range commit.StatusCheckRollup.Contexts.Nodes {
		state := c.State // StatusContext
		if state == "" {
			// CheckRun
			if c.Status == "COMPLETED" {
				state = c.Conclusion
			} else {
				state = c.Status
			}
		}
		switch state {
		case "SUCCESS", "NEUTRAL", "SKIPPED":
			summary.Passing++
		case "ERROR", "FAILURE", "CANCELLED", "TIMED_OUT", "ACTION_REQUIRED":
			summary.Failing++
		default: // "EXPECTED", "REQUESTED", "WAITING", "QUEUED", "PENDING", "IN_PROGRESS", "STALE"
			summary.Pending++
		}
		summary.Total++
	}
	return
}

func (pr *PullRequest) DisplayableReviews() PullRequestReviews {
	published := []PullRequestReview{}
	for _, prr := range pr.Reviews.Nodes {
		//Dont display pending reviews
		//Dont display commenting reviews without top level comment body
		if prr.State != "PENDING" && !(prr.State == "COMMENTED" && prr.Body == "") {
			published = append(published, prr)
		}
	}
	return PullRequestReviews{Nodes: published, TotalCount: len(published)}
}

// CreatePullRequest creates a pull request in a GitHub repository
func CreatePullRequest(client *Client, repo *Repository, params map[string]interface{}) (*PullRequest, error) {
	query := `
		mutation PullRequestCreate($input: CreatePullRequestInput!) {
			createPullRequest(input: $input) {
				pullRequest {
					id
					url
				}
			}
	}`

	inputParams := map[string]interface{}{
		"repositoryId": repo.ID,
	}
	for key, val := range params {
		switch key {
		case "title", "body", "draft", "baseRefName", "headRefName", "maintainerCanModify":
			inputParams[key] = val
		}
	}
	variables := map[string]interface{}{
		"input": inputParams,
	}

	result := struct {
		CreatePullRequest struct {
			PullRequest PullRequest
		}
	}{}

	err := client.GraphQL(repo.RepoHost(), query, variables, &result)
	if err != nil {
		return nil, err
	}
	pr := &result.CreatePullRequest.PullRequest

	// metadata parameters aren't currently available in `createPullRequest`,
	// but they are in `updatePullRequest`
	updateParams := make(map[string]interface{})
	for key, val := range params {
		switch key {
		case "assigneeIds", "labelIds", "projectIds", "milestoneId":
			if !isBlank(val) {
				updateParams[key] = val
			}
		}
	}
	if len(updateParams) > 0 {
		updateQuery := `
		mutation PullRequestCreateMetadata($input: UpdatePullRequestInput!) {
			updatePullRequest(input: $input) { clientMutationId }
		}`
		updateParams["pullRequestId"] = pr.ID
		variables := map[string]interface{}{
			"input": updateParams,
		}
		err := client.GraphQL(repo.RepoHost(), updateQuery, variables, &result)
		if err != nil {
			return pr, err
		}
	}

	// reviewers are requested in yet another additional mutation
	reviewParams := make(map[string]interface{})
	if ids, ok := params["userReviewerIds"]; ok && !isBlank(ids) {
		reviewParams["userIds"] = ids
	}
	if ids, ok := params["teamReviewerIds"]; ok && !isBlank(ids) {
		reviewParams["teamIds"] = ids
	}

	//TODO: How much work to extract this into own method and use for create and edit?
	if len(reviewParams) > 0 {
		reviewQuery := `
		mutation PullRequestCreateRequestReviews($input: RequestReviewsInput!) {
			requestReviews(input: $input) { clientMutationId }
		}`
		reviewParams["pullRequestId"] = pr.ID
		reviewParams["union"] = true
		variables := map[string]interface{}{
			"input": reviewParams,
		}
		err := client.GraphQL(repo.RepoHost(), reviewQuery, variables, &result)
		if err != nil {
			return pr, err
		}
	}

	return pr, nil
}

func UpdatePullRequestReviews(client *Client, repo ghrepo.Interface, params githubv4.RequestReviewsInput) error {
	var mutation struct {
		RequestReviews struct {
			PullRequest struct {
				ID string
			}
		} `graphql:"requestReviews(input: $input)"`
	}
	variables := map[string]interface{}{"input": params}
	gql := graphQLClient(client.http, repo.RepoHost())
	err := gql.MutateNamed(context.Background(), "PullRequestUpdateRequestReviews", &mutation, variables)
	return err
}

func isBlank(v interface{}) bool {
	switch vv := v.(type) {
	case string:
		return vv == ""
	case []string:
		return len(vv) == 0
	default:
		return true
	}
}

func PullRequestClose(httpClient *http.Client, repo ghrepo.Interface, prID string) error {
	var mutation struct {
		ClosePullRequest struct {
			PullRequest struct {
				ID githubv4.ID
			}
		} `graphql:"closePullRequest(input: $input)"`
	}

	variables := map[string]interface{}{
		"input": githubv4.ClosePullRequestInput{
			PullRequestID: prID,
		},
	}

	gql := graphQLClient(httpClient, repo.RepoHost())
	return gql.MutateNamed(context.Background(), "PullRequestClose", &mutation, variables)
}

func PullRequestReopen(httpClient *http.Client, repo ghrepo.Interface, prID string) error {
	var mutation struct {
		ReopenPullRequest struct {
			PullRequest struct {
				ID githubv4.ID
			}
		} `graphql:"reopenPullRequest(input: $input)"`
	}

	variables := map[string]interface{}{
		"input": githubv4.ReopenPullRequestInput{
			PullRequestID: prID,
		},
	}

	gql := graphQLClient(httpClient, repo.RepoHost())
	return gql.MutateNamed(context.Background(), "PullRequestReopen", &mutation, variables)
}

func PullRequestReady(client *Client, repo ghrepo.Interface, pr *PullRequest) error {
	var mutation struct {
		MarkPullRequestReadyForReview struct {
			PullRequest struct {
				ID githubv4.ID
			}
		} `graphql:"markPullRequestReadyForReview(input: $input)"`
	}

	variables := map[string]interface{}{
		"input": githubv4.MarkPullRequestReadyForReviewInput{
			PullRequestID: pr.ID,
		},
	}

	gql := graphQLClient(client.http, repo.RepoHost())
	return gql.MutateNamed(context.Background(), "PullRequestReadyForReview", &mutation, variables)
}

func BranchDeleteRemote(client *Client, repo ghrepo.Interface, branch string) error {
	path := fmt.Sprintf("repos/%s/%s/git/refs/heads/%s", repo.RepoOwner(), repo.RepoName(), branch)
	return client.REST(repo.RepoHost(), "DELETE", path, nil, nil)
}
