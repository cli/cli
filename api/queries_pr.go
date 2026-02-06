package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
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
	BaseRefOid          string
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
	Repository          *PRRepository
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
	AssignedActors AssignedActors
	Labels         Labels
	ProjectCards   ProjectCards
	ProjectItems   ProjectItems
	Milestone      *Milestone
	Comments       Comments
	ReactionGroups ReactionGroups
	Reviews        PullRequestReviews
	LatestReviews  PullRequestReviews
	ReviewRequests ReviewRequests

	ClosingIssuesReferences ClosingIssuesReferences
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

type ClosingIssuesReferences struct {
	Nodes []struct {
		ID         string
		Number     int
		URL        string
		Repository struct {
			ID    string
			Name  string
			Owner struct {
				ID    string
				Login string
			}
		}
	}
	PageInfo struct {
		HasNextPage bool
		EndCursor   string
	}
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
	ID            string `json:"id"`
	Name          string `json:"name"`
	NameWithOwner string `json:"nameWithOwner"`
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

const teamTypeName = "Team"
const botTypeName = "Bot"

func (r RequestedReviewer) LoginOrSlug() string {
	if r.TypeName == teamTypeName {
		return fmt.Sprintf("%s/%s", r.Organization.Login, r.Slug)
	}
	return r.Login
}

// DisplayName returns a user-friendly name for the reviewer.
// For Copilot bot, returns "Copilot (AI)". For teams, returns "org/slug".
// For users, returns "login (Name)" if name is available, otherwise just login.
func (r RequestedReviewer) DisplayName() string {
	if r.TypeName == teamTypeName {
		return fmt.Sprintf("%s/%s", r.Organization.Login, r.Slug)
	}
	if r.TypeName == botTypeName && r.Login == CopilotReviewerLogin {
		return "Copilot (AI)"
	}
	if r.Name != "" {
		return fmt.Sprintf("%s (%s)", r.Login, r.Name)
	}
	return r.Login
}

func (r ReviewRequests) Logins() []string {
	logins := make([]string, len(r.Nodes))
	for i, r := range r.Nodes {
		logins[i] = r.RequestedReviewer.LoginOrSlug()
	}
	return logins
}

// DisplayNames returns user-friendly display names for all requested reviewers.
func (r ReviewRequests) DisplayNames() []string {
	names := make([]string, len(r.Nodes))
	for i, r := range r.Nodes {
		names[i] = r.RequestedReviewer.DisplayName()
	}
	return names
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
						number
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

// extractTeamSlugs extracts just the slug portion from team identifiers.
// Team identifiers can be in "org/slug" format; this returns just the slug.
func extractTeamSlugs(teams []string) []string {
	slugs := make([]string, 0, len(teams))
	for _, t := range teams {
		if t == "" {
			continue
		}
		s := strings.SplitN(t, "/", 2)
		slugs = append(slugs, s[len(s)-1])
	}
	return slugs
}

// toGitHubV4Strings converts a string slice to a githubv4.String slice,
// optionally appending a suffix to each element.
func toGitHubV4Strings(strs []string, suffix string) []githubv4.String {
	result := make([]githubv4.String, len(strs))
	for i, s := range strs {
		result[i] = githubv4.String(s + suffix)
	}
	return result
}

// AddPullRequestReviews adds the given user and team reviewers to a pull request using the REST API.
// Team identifiers can be in "org/slug" format.
func AddPullRequestReviews(client *Client, repo ghrepo.Interface, prNumber int, users, teams []string) error {
	if len(users) == 0 && len(teams) == 0 {
		return nil
	}

	// The API requires empty arrays instead of null values
	if users == nil {
		users = []string{}
	}

	path := fmt.Sprintf(
		"repos/%s/%s/pulls/%d/requested_reviewers",
		url.PathEscape(repo.RepoOwner()),
		url.PathEscape(repo.RepoName()),
		prNumber,
	)
	body := struct {
		Reviewers     []string `json:"reviewers"`
		TeamReviewers []string `json:"team_reviewers"`
	}{
		Reviewers:     users,
		TeamReviewers: extractTeamSlugs(teams),
	}
	buf := &bytes.Buffer{}
	if err := json.NewEncoder(buf).Encode(body); err != nil {
		return err
	}
	// The endpoint responds with the updated pull request object; we don't need it here.
	return client.REST(repo.RepoHost(), "POST", path, buf, nil)
}

// RemovePullRequestReviews removes requested reviewers from a pull request using the REST API.
// Team identifiers can be in "org/slug" format.
func RemovePullRequestReviews(client *Client, repo ghrepo.Interface, prNumber int, users, teams []string) error {
	if len(users) == 0 && len(teams) == 0 {
		return nil
	}

	// The API requires empty arrays instead of null values
	if users == nil {
		users = []string{}
	}

	path := fmt.Sprintf(
		"repos/%s/%s/pulls/%d/requested_reviewers",
		url.PathEscape(repo.RepoOwner()),
		url.PathEscape(repo.RepoName()),
		prNumber,
	)
	body := struct {
		Reviewers     []string `json:"reviewers"`
		TeamReviewers []string `json:"team_reviewers"`
	}{
		Reviewers:     users,
		TeamReviewers: extractTeamSlugs(teams),
	}
	buf := &bytes.Buffer{}
	if err := json.NewEncoder(buf).Encode(body); err != nil {
		return err
	}
	// The endpoint responds with the updated pull request object; we don't need it here.
	return client.REST(repo.RepoHost(), "DELETE", path, buf, nil)
}

// RequestReviewsByLogin sets requested reviewers on a pull request using the GraphQL mutation.
// This mutation replaces existing reviewers with the provided set unless union is true.
// Only available on github.com, not GHES.
// Bot logins should include the [bot] suffix (e.g., "copilot-pull-request-reviewer[bot]").
// Team slugs should be in the format "org/team-slug".
// When union is false (replace mode), passing empty slices will remove all reviewers.
func RequestReviewsByLogin(client *Client, repo ghrepo.Interface, prID string, userLogins, botLogins, teamSlugs []string, union bool) error {
	// In union mode (additive), nothing to do if all lists are empty.
	// In replace mode, we may still need to call the mutation to clear reviewers.
	if union && len(userLogins) == 0 && len(botLogins) == 0 && len(teamSlugs) == 0 {
		return nil
	}

	var mutation struct {
		RequestReviewsByLogin struct {
			ClientMutationId string `graphql:"clientMutationId"`
		} `graphql:"requestReviewsByLogin(input: $input)"`
	}

	type RequestReviewsByLoginInput struct {
		PullRequestID githubv4.ID        `json:"pullRequestId"`
		UserLogins    *[]githubv4.String `json:"userLogins,omitempty"`
		BotLogins     *[]githubv4.String `json:"botLogins,omitempty"`
		TeamSlugs     *[]githubv4.String `json:"teamSlugs,omitempty"`
		Union         githubv4.Boolean   `json:"union"`
	}

	input := RequestReviewsByLoginInput{
		PullRequestID: githubv4.ID(prID),
		Union:         githubv4.Boolean(union),
	}

	userLoginValues := toGitHubV4Strings(userLogins, "")
	input.UserLogins = &userLoginValues

	// Bot logins require the [bot] suffix for the mutation
	botLoginValues := toGitHubV4Strings(botLogins, "[bot]")
	input.BotLogins = &botLoginValues

	teamSlugValues := toGitHubV4Strings(teamSlugs, "")
	input.TeamSlugs = &teamSlugValues

	variables := map[string]interface{}{
		"input": input,
	}

	return client.Mutate(repo.RepoHost(), "RequestReviewsByLogin", &mutation, variables)
}

// SuggestedAssignableActors fetches up to 10 suggested actors for a specific assignable
// (Issue or PullRequest) node ID. `assignableID` is the GraphQL node ID for the Issue/PR.
// Returns the actors, the total count of available assignees in the repo, and an error.
func SuggestedAssignableActors(client *Client, repo ghrepo.Interface, assignableID string, query string) ([]AssignableActor, int, error) {
	type responseData struct {
		Repository struct {
			AssignableUsers struct {
				TotalCount int
			}
		} `graphql:"repository(owner: $owner, name: $name)"`
		Node struct {
			Issue struct {
				SuggestedActors struct {
					Nodes []struct {
						TypeName string `graphql:"__typename"`
						User     struct {
							ID    string
							Login string
							Name  string
						} `graphql:"... on User"`
						Bot struct {
							ID    string
							Login string
						} `graphql:"... on Bot"`
					}
				} `graphql:"suggestedActors(first: 10, query: $query)"`
			} `graphql:"... on Issue"`
			PullRequest struct {
				SuggestedActors struct {
					Nodes []struct {
						TypeName string `graphql:"__typename"`
						User     struct {
							ID    string
							Login string
							Name  string
						} `graphql:"... on User"`
						Bot struct {
							ID    string
							Login string
						} `graphql:"... on Bot"`
					}
				} `graphql:"suggestedActors(first: 10, query: $query)"`
			} `graphql:"... on PullRequest"`
		} `graphql:"node(id: $id)"`
	}

	variables := map[string]interface{}{
		"id":    githubv4.ID(assignableID),
		"query": githubv4.String(query),
		"owner": githubv4.String(repo.RepoOwner()),
		"name":  githubv4.String(repo.RepoName()),
	}

	var result responseData
	if err := client.Query(repo.RepoHost(), "SuggestedAssignableActors", &result, variables); err != nil {
		return nil, 0, err
	}

	availableAssigneesCount := result.Repository.AssignableUsers.TotalCount

	var nodes []struct {
		TypeName string `graphql:"__typename"`
		User     struct {
			ID    string
			Login string
			Name  string
		} `graphql:"... on User"`
		Bot struct {
			ID    string
			Login string
		} `graphql:"... on Bot"`
	}

	if result.Node.PullRequest.SuggestedActors.Nodes != nil {
		nodes = result.Node.PullRequest.SuggestedActors.Nodes
	} else if result.Node.Issue.SuggestedActors.Nodes != nil {
		nodes = result.Node.Issue.SuggestedActors.Nodes
	}

	actors := make([]AssignableActor, 0, len(nodes))

	for _, n := range nodes {
		if n.TypeName == "User" && n.User.Login != "" {
			actors = append(actors, AssignableUser{id: n.User.ID, login: n.User.Login, name: n.User.Name})
		} else if n.TypeName == "Bot" && n.Bot.Login != "" {
			actors = append(actors, AssignableBot{id: n.Bot.ID, login: n.Bot.Login})
		}
	}

	return actors, availableAssigneesCount, nil
}

// ReviewerCandidate represents a potential reviewer for a pull request.
// This can be a User, Bot, or Team.
type ReviewerCandidate interface {
	DisplayName() string
	Login() string

	sealedReviewerCandidate()
}

// ReviewerUser is a user who can review a pull request.
type ReviewerUser struct {
	AssignableUser
}

func NewReviewerUser(login, name string) ReviewerUser {
	return ReviewerUser{
		AssignableUser: NewAssignableUser("", login, name),
	}
}

func (r ReviewerUser) sealedReviewerCandidate() {}

// ReviewerBot is a bot who can review a pull request.
type ReviewerBot struct {
	AssignableBot
}

func NewReviewerBot(login string) ReviewerBot {
	return ReviewerBot{
		AssignableBot: NewAssignableBot("", login),
	}
}

func (b ReviewerBot) DisplayName() string {
	if b.login == CopilotReviewerLogin {
		return fmt.Sprintf("%s (AI)", CopilotActorName)
	}
	return b.Login()
}

func (r ReviewerBot) sealedReviewerCandidate() {}

// ReviewerTeam is a team that can review a pull request.
type ReviewerTeam struct {
	org      string
	teamSlug string
}

// NewReviewerTeam creates a new ReviewerTeam.
func NewReviewerTeam(orgName, teamSlug string) ReviewerTeam {
	return ReviewerTeam{org: orgName, teamSlug: teamSlug}
}

func (r ReviewerTeam) DisplayName() string {
	return fmt.Sprintf("%s/%s", r.org, r.teamSlug)
}

func (r ReviewerTeam) Login() string {
	return fmt.Sprintf("%s/%s", r.org, r.teamSlug)
}

func (r ReviewerTeam) Slug() string {
	return r.teamSlug
}

func (r ReviewerTeam) sealedReviewerCandidate() {}

// SuggestedReviewerActors fetches suggested reviewers for a pull request.
// It combines results from three sources using a cascading quota system:
// - suggestedReviewerActors - suggested based on PR activity (base quota: 5)
// - repository collaborators - all collaborators (base quota: 5 + unfilled from suggestions)
// - organization teams - all teams for org repos (base quota: 5 + unfilled from collaborators)
//
// This ensures we show up to 15 total candidates, with each source filling any
// unfilled quota from the previous source. Results are deduplicated.
// Returns the candidates, a MoreResults count, and an error.
func SuggestedReviewerActors(client *Client, repo ghrepo.Interface, prID string, query string) ([]ReviewerCandidate, int, error) {
	// Fetch 10 from each source to allow cascading quota to fill from available results.
	// Use a single query that includes organization.teams - if the owner is not an org,
	// we'll get a "Could not resolve to an Organization" error which we handle gracefully.
	// We also fetch unfiltered total counts via aliases for the "X more" display.
	type responseData struct {
		Node struct {
			PullRequest struct {
				SuggestedActors struct {
					Nodes []struct {
						IsAuthor    bool
						IsCommenter bool
						Reviewer    struct {
							TypeName string `graphql:"__typename"`
							User     struct {
								Login string
								Name  string
							} `graphql:"... on User"`
							Bot struct {
								Login string
							} `graphql:"... on Bot"`
						}
					}
				} `graphql:"suggestedReviewerActors(first: 10, query: $query)"`
			} `graphql:"... on PullRequest"`
		} `graphql:"node(id: $id)"`
		Repository struct {
			Collaborators struct {
				Nodes []struct {
					Login string
					Name  string
				}
			} `graphql:"collaborators(first: 10, query: $query)"`
			CollaboratorsTotalCount struct {
				TotalCount int
			} `graphql:"collaboratorsTotalCount: collaborators(first: 0)"`
		} `graphql:"repository(owner: $owner, name: $name)"`
		Organization struct {
			Teams struct {
				Nodes []struct {
					Slug string
				}
			} `graphql:"teams(first: 10, query: $query)"`
			TeamsTotalCount struct {
				TotalCount int
			} `graphql:"teamsTotalCount: teams(first: 0)"`
		} `graphql:"organization(login: $owner)"`
	}

	variables := map[string]interface{}{
		"id":    githubv4.ID(prID),
		"query": githubv4.String(query),
		"owner": githubv4.String(repo.RepoOwner()),
		"name":  githubv4.String(repo.RepoName()),
	}

	var result responseData
	err := client.Query(repo.RepoHost(), "SuggestedReviewerActors", &result, variables)
	// Handle the case where the owner is not an organization - the query still returns
	// partial data (repository, node), so we can continue processing.
	if err != nil && !strings.Contains(err.Error(), errorResolvingOrganization) {
		return nil, 0, err
	}

	// Build candidates using cascading quota logic:
	// Each source has a base quota of 5, plus any unfilled quota from previous sources.
	// This ensures we show up to 15 total candidates, filling gaps when earlier sources have fewer.
	seen := make(map[string]bool)
	var candidates []ReviewerCandidate
	const baseQuota = 5

	// Suggested reviewers (excluding author)
	suggestionsAdded := 0
	for _, n := range result.Node.PullRequest.SuggestedActors.Nodes {
		if suggestionsAdded >= baseQuota {
			break
		}
		if n.IsAuthor {
			continue
		}
		var candidate ReviewerCandidate
		var login string
		if n.Reviewer.TypeName == "User" && n.Reviewer.User.Login != "" {
			login = n.Reviewer.User.Login
			candidate = NewReviewerUser(login, n.Reviewer.User.Name)
		} else if n.Reviewer.TypeName == "Bot" && n.Reviewer.Bot.Login != "" {
			login = n.Reviewer.Bot.Login
			candidate = NewReviewerBot(login)
		} else {
			continue
		}
		if !seen[login] {
			seen[login] = true
			candidates = append(candidates, candidate)
			suggestionsAdded++
		}
	}

	// Collaborators: quota = base + unfilled from suggestions
	collaboratorsQuota := baseQuota + (baseQuota - suggestionsAdded)
	collaboratorsAdded := 0
	for _, c := range result.Repository.Collaborators.Nodes {
		if collaboratorsAdded >= collaboratorsQuota {
			break
		}
		if c.Login == "" {
			continue
		}
		if !seen[c.Login] {
			seen[c.Login] = true
			candidates = append(candidates, NewReviewerUser(c.Login, c.Name))
			collaboratorsAdded++
		}
	}

	// Teams: quota = base + unfilled from collaborators
	teamsQuota := baseQuota + (collaboratorsQuota - collaboratorsAdded)
	teamsAdded := 0
	ownerName := repo.RepoOwner()
	for _, t := range result.Organization.Teams.Nodes {
		if teamsAdded >= teamsQuota {
			break
		}
		if t.Slug == "" {
			continue
		}
		teamLogin := fmt.Sprintf("%s/%s", ownerName, t.Slug)
		if !seen[teamLogin] {
			seen[teamLogin] = true
			candidates = append(candidates, NewReviewerTeam(ownerName, t.Slug))
			teamsAdded++
		}
	}

	// MoreResults uses unfiltered total counts (teams will be 0 for personal repos)
	moreResults := result.Repository.CollaboratorsTotalCount.TotalCount + result.Organization.TeamsTotalCount.TotalCount

	return candidates, moreResults, nil
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
			RevertPullRequest struct {
				ID     string
				Number int
				URL    string
			}
		} `graphql:"revertPullRequest(input: $input)"`
	}

	variables := map[string]interface{}{
		"input": params,
	}
	err := client.Mutate(repo.RepoHost(), "PullRequestRevert", &mutation, variables)
	if err != nil {
		return nil, err
	}
	pr := &mutation.RevertPullRequest.RevertPullRequest
	revertPR := &PullRequest{
		ID:     pr.ID,
		Number: pr.Number,
		URL:    pr.URL,
	}

	return revertPR, nil
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
