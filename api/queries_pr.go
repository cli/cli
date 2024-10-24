package api

import (
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/shurcooL/githubv4"
)

type PullRequestAndTotalCount struct {
	TotalCount   int
	PullRequests []PullRequest
	SearchCapped bool
}

type PullRequestMergeable string

const (
	PullRequestMergeableConflicting PullRequestMergeable = "CONFLICTING"
	PullRequestMergeableMergeable   PullRequestMergeable = "MERGEABLE"
	PullRequestMergeableUnknown     PullRequestMergeable = "UNKNOWN"
)

type PullRequest struct {
	ID                  string
	FullDatabaseID      string
	Number              int
	Title               string
	State               string
	Closed              bool
	URL                 string
	BaseRefName         string
	HeadRefName         string
	HeadRefOid          string
	Body                string
	Mergeable           PullRequestMergeable
	Additions           int
	Deletions           int
	ChangedFiles        int
	MergeStateStatus    string
	IsInMergeQueue      bool
	IsMergeQueueEnabled bool // Indicates whether the pull request's base ref has a merge queue enabled.
	CreatedAt           time.Time
	UpdatedAt           time.Time
	ClosedAt            *time.Time
	MergedAt            *time.Time

	AutoMergeRequest *AutoMergeRequest

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
		Nodes []StatusCheckRollupNode
	}

	Assignees      Assignees
	Labels         Labels
	ProjectCards   ProjectCards
	ProjectItems   ProjectItems
	Milestone      *Milestone
	Comments       Comments
	ReactionGroups ReactionGroups
	Reviews        PullRequestReviews
	LatestReviews  PullRequestReviews
	ReviewRequests ReviewRequests
}

type StatusCheckRollupNode struct {
	Commit StatusCheckRollupCommit
}

type StatusCheckRollupCommit struct {
	StatusCheckRollup CommitStatusCheckRollup
}

type CommitStatusCheckRollup struct {
	Contexts CheckContexts
}

// https://docs.github.com/en/graphql/reference/enums#checkrunstate
type CheckRunState string

const (
	CheckRunStateActionRequired CheckRunState = "ACTION_REQUIRED"
	CheckRunStateCancelled      CheckRunState = "CANCELLED"
	CheckRunStateCompleted      CheckRunState = "COMPLETED"
	CheckRunStateFailure        CheckRunState = "FAILURE"
	CheckRunStateInProgress     CheckRunState = "IN_PROGRESS"
	CheckRunStateNeutral        CheckRunState = "NEUTRAL"
	CheckRunStatePending        CheckRunState = "PENDING"
	CheckRunStateQueued         CheckRunState = "QUEUED"
	CheckRunStateSkipped        CheckRunState = "SKIPPED"
	CheckRunStateStale          CheckRunState = "STALE"
	CheckRunStateStartupFailure CheckRunState = "STARTUP_FAILURE"
	CheckRunStateSuccess        CheckRunState = "SUCCESS"
	CheckRunStateTimedOut       CheckRunState = "TIMED_OUT"
	CheckRunStateWaiting        CheckRunState = "WAITING"
)

type CheckRunCountByState struct {
	State CheckRunState
	Count int
}

// https://docs.github.com/en/graphql/reference/enums#statusstate
type StatusState string

const (
	StatusStateError    StatusState = "ERROR"
	StatusStateExpected StatusState = "EXPECTED"
	StatusStateFailure  StatusState = "FAILURE"
	StatusStatePending  StatusState = "PENDING"
	StatusStateSuccess  StatusState = "SUCCESS"
)

type StatusContextCountByState struct {
	State StatusState
	Count int
}

// https://docs.github.com/en/graphql/reference/enums#checkstatusstate
type CheckStatusState string

const (
	CheckStatusStateCompleted  CheckStatusState = "COMPLETED"
	CheckStatusStateInProgress CheckStatusState = "IN_PROGRESS"
	CheckStatusStatePending    CheckStatusState = "PENDING"
	CheckStatusStateQueued     CheckStatusState = "QUEUED"
	CheckStatusStateRequested  CheckStatusState = "REQUESTED"
	CheckStatusStateWaiting    CheckStatusState = "WAITING"
)

// https://docs.github.com/en/graphql/reference/enums#checkconclusionstate
type CheckConclusionState string

const (
	CheckConclusionStateActionRequired CheckConclusionState = "ACTION_REQUIRED"
	CheckConclusionStateCancelled      CheckConclusionState = "CANCELLED"
	CheckConclusionStateFailure        CheckConclusionState = "FAILURE"
	CheckConclusionStateNeutral        CheckConclusionState = "NEUTRAL"
	CheckConclusionStateSkipped        CheckConclusionState = "SKIPPED"
	CheckConclusionStateStale          CheckConclusionState = "STALE"
	CheckConclusionStateStartupFailure CheckConclusionState = "STARTUP_FAILURE"
	CheckConclusionStateSuccess        CheckConclusionState = "SUCCESS"
	CheckConclusionStateTimedOut       CheckConclusionState = "TIMED_OUT"
)

type CheckContexts struct {
	// These fields are available on newer versions of the GraphQL API
	// to support summary counts by state
	CheckRunCount              int
	CheckRunCountsByState      []CheckRunCountByState
	StatusContextCount         int
	StatusContextCountsByState []StatusContextCountByState

	// These are available on older versions and provide more details
	// required for checks
	Nodes    []CheckContext
	PageInfo struct {
		HasNextPage bool
		EndCursor   string
	}
}

type CheckContext struct {
	TypeName   string     `json:"__typename"`
	Name       string     `json:"name"`
	IsRequired bool       `json:"isRequired"`
	CheckSuite CheckSuite `json:"checkSuite"`
	// QUEUED IN_PROGRESS COMPLETED WAITING PENDING REQUESTED
	Status string `json:"status"`
	// ACTION_REQUIRED TIMED_OUT CANCELLED FAILURE SUCCESS NEUTRAL SKIPPED STARTUP_FAILURE STALE
	Conclusion  CheckConclusionState `json:"conclusion"`
	StartedAt   time.Time            `json:"startedAt"`
	CompletedAt time.Time            `json:"completedAt"`
	DetailsURL  string               `json:"detailsUrl"`

	/* StatusContext fields */
	Context     string `json:"context"`
	Description string `json:"description"`
	// EXPECTED ERROR FAILURE PENDING SUCCESS
	State     StatusState `json:"state"`
	TargetURL string      `json:"targetUrl"`
	CreatedAt time.Time   `json:"createdAt"`
}

type CheckSuite struct {
	WorkflowRun WorkflowRun `json:"workflowRun"`
}

type WorkflowRun struct {
	Event    string   `json:"event"`
	Workflow Workflow `json:"workflow"`
}

type Workflow struct {
	Name string `json:"name"`
}

type PRRepository struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type AutoMergeRequest struct {
	AuthorEmail    *string `json:"authorEmail"`
	CommitBody     *string `json:"commitBody"`
	CommitHeadline *string `json:"commitHeadline"`
	// MERGE, REBASE, SQUASH
	MergeMethod string    `json:"mergeMethod"`
	EnabledAt   time.Time `json:"enabledAt"`
	EnabledBy   Author    `json:"enabledBy"`
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

func (pr PullRequest) CurrentUserComments() []Comment {
	return pr.Comments.CurrentUserComments()
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

func (pr *PullRequest) ChecksStatus() PullRequestChecksStatus {
	var summary PullRequestChecksStatus

	if len(pr.StatusCheckRollup.Nodes) == 0 {
		return summary
	}

	contexts := pr.StatusCheckRollup.Nodes[0].Commit.StatusCheckRollup.Contexts

	// If this commit has counts by state then we can summarise check status from those
	if len(contexts.CheckRunCountsByState) != 0 && len(contexts.StatusContextCountsByState) != 0 {
		summary.Total = contexts.CheckRunCount + contexts.StatusContextCount
		for _, countByState := range contexts.CheckRunCountsByState {
			switch parseCheckStatusFromCheckRunState(countByState.State) {
			case passing:
				summary.Passing += countByState.Count
			case failing:
				summary.Failing += countByState.Count
			default:
				summary.Pending += countByState.Count
			}
		}

		for _, countByState := range contexts.StatusContextCountsByState {
			switch parseCheckStatusFromStatusState(countByState.State) {
			case passing:
				summary.Passing += countByState.Count
			case failing:
				summary.Failing += countByState.Count
			default:
				summary.Pending += countByState.Count
			}
		}

		return summary
	}

	// If we don't have the counts by state, then we'll need to summarise by looking at the more detailed contexts
	for _, c := range contexts.Nodes {
		// Nodes are a discriminated union of CheckRun or StatusContext and we can match on
		// the TypeName to narrow the type.
		if c.TypeName == "CheckRun" {
			// https://docs.github.com/en/graphql/reference/enums#checkstatusstate
			// If the status is completed then we can check the conclusion field
			if c.Status == "COMPLETED" {
				switch parseCheckStatusFromCheckConclusionState(c.Conclusion) {
				case passing:
					summary.Passing++
				case failing:
					summary.Failing++
				default:
					summary.Pending++
				}
				// otherwise we're in some form of pending state:
				// "COMPLETED", "IN_PROGRESS", "PENDING", "QUEUED", "REQUESTED", "WAITING" or otherwise unknown
			} else {
				summary.Pending++
			}

		} else { // c.TypeName == StatusContext
			switch parseCheckStatusFromStatusState(c.State) {
			case passing:
				summary.Passing++
			case failing:
				summary.Failing++
			default:
				summary.Pending++
			}
		}
		summary.Total++
	}

	return summary
}

type checkStatus int

const (
	passing checkStatus = iota
	failing
	pending
)

func parseCheckStatusFromStatusState(state StatusState) checkStatus {
	switch state {
	case StatusStateSuccess:
		return passing
	case StatusStateFailure, StatusStateError:
		return failing
	case StatusStateExpected, StatusStatePending:
		return pending
	// Currently, we treat anything unknown as pending, which includes any future unknown
	// states we might get back from the API. It might be interesting to do some work to add an additional
	// unknown state.
	default:
		return pending
	}
}

func parseCheckStatusFromCheckRunState(state CheckRunState) checkStatus {
	switch state {
	case CheckRunStateNeutral, CheckRunStateSkipped, CheckRunStateSuccess:
		return passing
	case CheckRunStateActionRequired, CheckRunStateCancelled, CheckRunStateFailure, CheckRunStateTimedOut:
		return failing
	case CheckRunStateCompleted, CheckRunStateInProgress, CheckRunStatePending, CheckRunStateQueued,
		CheckRunStateStale, CheckRunStateStartupFailure, CheckRunStateWaiting:
		return pending
	// Currently, we treat anything unknown as pending, which includes any future unknown
	// states we might get back from the API. It might be interesting to do some work to add an additional
	// unknown state.
	default:
		return pending
	}
}

func parseCheckStatusFromCheckConclusionState(state CheckConclusionState) checkStatus {
	switch state {
	case CheckConclusionStateNeutral, CheckConclusionStateSkipped, CheckConclusionStateSuccess:
		return passing
	case CheckConclusionStateActionRequired, CheckConclusionStateCancelled, CheckConclusionStateFailure, CheckConclusionStateTimedOut:
		return failing
	case CheckConclusionStateStale, CheckConclusionStateStartupFailure:
		return pending
	// Currently, we treat anything unknown as pending, which includes any future unknown
	// states we might get back from the API. It might be interesting to do some work to add an additional
	// unknown state.
	default:
		return pending
	}
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

	// projectsV2 are added in yet another mutation
	projectV2Ids, ok := params["projectV2Ids"].([]string)
	if ok {
		projectItems := make(map[string]string, len(projectV2Ids))
		for _, p := range projectV2Ids {
			projectItems[p] = pr.ID
		}
		err = UpdateProjectV2Items(client, repo, projectItems, nil)
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
	err := client.Mutate(repo.RepoHost(), "PullRequestUpdateRequestReviews", &mutation, variables)
	return err
}

func UpdatePullRequestBranch(client *Client, repo ghrepo.Interface, params githubv4.UpdatePullRequestBranchInput) error {
	var mutation struct {
		UpdatePullRequestBranch struct {
			PullRequest struct {
				ID string
			}
		} `graphql:"updatePullRequestBranch(input: $input)"`
	}
	variables := map[string]interface{}{"input": params}
	return client.Mutate(repo.RepoHost(), "PullRequestUpdateBranch", &mutation, variables)
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

	client := NewClientFromHTTP(httpClient)
	return client.Mutate(repo.RepoHost(), "PullRequestClose", &mutation, variables)
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

	client := NewClientFromHTTP(httpClient)
	return client.Mutate(repo.RepoHost(), "PullRequestReopen", &mutation, variables)
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

	return client.Mutate(repo.RepoHost(), "PullRequestReadyForReview", &mutation, variables)
}

func PullRequestRevert(client *Client, repo ghrepo.Interface, params githubv4.RevertPullRequestInput) (*PullRequest, error) {
	var mutation struct {
		RevertPullRequest struct {
			PullRequest struct {
				ID githubv4.ID
			}
			RevertPullRequest PullRequest
		} `graphql:"revertPullRequest(input: $input)"`
	}

	variables := map[string]interface{}{
		"input": params,
	}
	err := client.Mutate(repo.RepoHost(), "PullRequestRevert", &mutation, variables)
	if err != nil {
		return nil, err
	}
	revertPR := &mutation.RevertPullRequest.RevertPullRequest
	return revertPR, err
}

func ConvertPullRequestToDraft(client *Client, repo ghrepo.Interface, pr *PullRequest) error {
	var mutation struct {
		ConvertPullRequestToDraft struct {
			PullRequest struct {
				ID githubv4.ID
			}
		} `graphql:"convertPullRequestToDraft(input: $input)"`
	}

	variables := map[string]interface{}{
		"input": githubv4.ConvertPullRequestToDraftInput{
			PullRequestID: pr.ID,
		},
	}

	return client.Mutate(repo.RepoHost(), "ConvertPullRequestToDraft", &mutation, variables)
}

func BranchDeleteRemote(client *Client, repo ghrepo.Interface, branch string) error {
	path := fmt.Sprintf("repos/%s/%s/git/refs/heads/%s", repo.RepoOwner(), repo.RepoName(), url.PathEscape(branch))
	return client.REST(repo.RepoHost(), "DELETE", path, nil, nil)
}

type RefComparison struct {
	AheadBy  int
	BehindBy int
	Status   string
}

func ComparePullRequestBaseBranchWith(client *Client, repo ghrepo.Interface, prNumber int, headRef string) (*RefComparison, error) {
	query := `query ComparePullRequestBaseBranchWith($owner: String!, $repo: String!, $pullRequestNumber: Int!, $headRef: String!) {
		repository(owner: $owner, name: $repo) {
			pullRequest(number: $pullRequestNumber) {
				baseRef {
					compare (headRef: $headRef) {
						aheadBy, behindBy, status
					}
				}
			}
		}
	}`

	var result struct {
		Repository struct {
			PullRequest struct {
				BaseRef struct {
					Compare RefComparison
				}
			}
		}
	}
	variables := map[string]interface{}{
		"owner":             repo.RepoOwner(),
		"repo":              repo.RepoName(),
		"pullRequestNumber": prNumber,
		"headRef":           headRef,
	}

	if err := client.GraphQL(repo.RepoHost(), query, variables, &result); err != nil {
		return nil, err
	}
	return &result.Repository.PullRequest.BaseRef.Compare, nil
}
