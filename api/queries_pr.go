package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cli/cli/internal/ghinstance"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/set"
	"github.com/shurcooL/githubv4"
	"golang.org/x/sync/errgroup"
)

type PullRequestsPayload struct {
	ViewerCreated   PullRequestAndTotalCount
	ReviewRequested PullRequestAndTotalCount
	CurrentPR       *PullRequest
	DefaultBranch   string
}

type PullRequestAndTotalCount struct {
	TotalCount   int
	PullRequests []PullRequest
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
			RequiresStrictStatusChecks bool
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
						Nodes []struct {
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
	ReviewRequests ReviewRequests
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

func (c Client) PullRequestDiff(baseRepo ghrepo.Interface, prNumber int) (io.ReadCloser, error) {
	url := fmt.Sprintf("%srepos/%s/pulls/%d",
		ghinstance.RESTPrefix(baseRepo.RepoHost()), ghrepo.FullName(baseRepo), prNumber)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/vnd.github.v3.diff; charset=utf-8")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == 404 {
		return nil, errors.New("pull request not found")
	} else if resp.StatusCode != 200 {
		return nil, HandleHTTPError(resp)
	}

	return resp.Body, nil
}

type pullRequestFeature struct {
	HasReviewDecision       bool
	HasStatusCheckRollup    bool
	HasBranchProtectionRule bool
}

func determinePullRequestFeatures(httpClient *http.Client, hostname string) (prFeatures pullRequestFeature, err error) {
	if !ghinstance.IsEnterprise(hostname) {
		prFeatures.HasReviewDecision = true
		prFeatures.HasStatusCheckRollup = true
		prFeatures.HasBranchProtectionRule = true
		return
	}

	var featureDetection struct {
		PullRequest struct {
			Fields []struct {
				Name string
			} `graphql:"fields(includeDeprecated: true)"`
		} `graphql:"PullRequest: __type(name: \"PullRequest\")"`
		Commit struct {
			Fields []struct {
				Name string
			} `graphql:"fields(includeDeprecated: true)"`
		} `graphql:"Commit: __type(name: \"Commit\")"`
	}

	// needs to be a separate query because the backend only supports 2 `__type` expressions in one query
	var featureDetection2 struct {
		Ref struct {
			Fields []struct {
				Name string
			} `graphql:"fields(includeDeprecated: true)"`
		} `graphql:"Ref: __type(name: \"Ref\")"`
	}

	v4 := graphQLClient(httpClient, hostname)

	g := new(errgroup.Group)
	g.Go(func() error {
		return v4.QueryNamed(context.Background(), "PullRequest_fields", &featureDetection, nil)
	})
	g.Go(func() error {
		return v4.QueryNamed(context.Background(), "PullRequest_fields2", &featureDetection2, nil)
	})

	err = g.Wait()
	if err != nil {
		return
	}

	for _, field := range featureDetection.PullRequest.Fields {
		switch field.Name {
		case "reviewDecision":
			prFeatures.HasReviewDecision = true
		}
	}
	for _, field := range featureDetection.Commit.Fields {
		switch field.Name {
		case "statusCheckRollup":
			prFeatures.HasStatusCheckRollup = true
		}
	}
	for _, field := range featureDetection2.Ref.Fields {
		switch field.Name {
		case "branchProtectionRule":
			prFeatures.HasBranchProtectionRule = true
		}
	}
	return
}

type StatusOptions struct {
	CurrentPR int
	HeadRef   string
	Username  string
	Fields    []string
}

func PullRequestStatus(client *Client, repo ghrepo.Interface, options StatusOptions) (*PullRequestsPayload, error) {
	type edges struct {
		TotalCount int
		Edges      []struct {
			Node PullRequest
		}
	}

	type response struct {
		Repository struct {
			DefaultBranchRef struct {
				Name string
			}
			PullRequests edges
			PullRequest  *PullRequest
		}
		ViewerCreated   edges
		ReviewRequested edges
	}

	var fragments string
	if len(options.Fields) > 0 {
		fields := set.NewStringSet()
		fields.AddValues(options.Fields)
		// these are always necessary to find the PR for the current branch
		fields.AddValues([]string{"isCrossRepository", "headRepositoryOwner", "headRefName"})
		gr := PullRequestGraphQL(fields.ToSlice())
		fragments = fmt.Sprintf("fragment pr on PullRequest{%[1]s}fragment prWithReviews on PullRequest{%[1]s}", gr)
	} else {
		var err error
		fragments, err = pullRequestFragment(client.http, repo.RepoHost())
		if err != nil {
			return nil, err
		}
	}

	queryPrefix := `
	query PullRequestStatus($owner: String!, $repo: String!, $headRefName: String!, $viewerQuery: String!, $reviewerQuery: String!, $per_page: Int = 10) {
		repository(owner: $owner, name: $repo) {
			defaultBranchRef {
				name
			}
			pullRequests(headRefName: $headRefName, first: $per_page, orderBy: { field: CREATED_AT, direction: DESC }) {
				totalCount
				edges {
					node {
						...prWithReviews
					}
				}
			}
		}
	`
	if options.CurrentPR > 0 {
		queryPrefix = `
		query PullRequestStatus($owner: String!, $repo: String!, $number: Int!, $viewerQuery: String!, $reviewerQuery: String!, $per_page: Int = 10) {
			repository(owner: $owner, name: $repo) {
				defaultBranchRef {
					name
				}
				pullRequest(number: $number) {
					...prWithReviews
				}
			}
		`
	}

	query := fragments + queryPrefix + `
      viewerCreated: search(query: $viewerQuery, type: ISSUE, first: $per_page) {
       totalCount: issueCount
        edges {
          node {
            ...prWithReviews
          }
        }
      }
      reviewRequested: search(query: $reviewerQuery, type: ISSUE, first: $per_page) {
        totalCount: issueCount
        edges {
          node {
            ...pr
          }
        }
      }
    }
	`

	currentUsername := options.Username
	if currentUsername == "@me" && ghinstance.IsEnterprise(repo.RepoHost()) {
		var err error
		currentUsername, err = CurrentLoginName(client, repo.RepoHost())
		if err != nil {
			return nil, err
		}
	}

	viewerQuery := fmt.Sprintf("repo:%s state:open is:pr author:%s", ghrepo.FullName(repo), currentUsername)
	reviewerQuery := fmt.Sprintf("repo:%s state:open review-requested:%s", ghrepo.FullName(repo), currentUsername)

	currentPRHeadRef := options.HeadRef
	branchWithoutOwner := currentPRHeadRef
	if idx := strings.Index(currentPRHeadRef, ":"); idx >= 0 {
		branchWithoutOwner = currentPRHeadRef[idx+1:]
	}

	variables := map[string]interface{}{
		"viewerQuery":   viewerQuery,
		"reviewerQuery": reviewerQuery,
		"owner":         repo.RepoOwner(),
		"repo":          repo.RepoName(),
		"headRefName":   branchWithoutOwner,
		"number":        options.CurrentPR,
	}

	var resp response
	err := client.GraphQL(repo.RepoHost(), query, variables, &resp)
	if err != nil {
		return nil, err
	}

	var viewerCreated []PullRequest
	for _, edge := range resp.ViewerCreated.Edges {
		viewerCreated = append(viewerCreated, edge.Node)
	}

	var reviewRequested []PullRequest
	for _, edge := range resp.ReviewRequested.Edges {
		reviewRequested = append(reviewRequested, edge.Node)
	}

	var currentPR = resp.Repository.PullRequest
	if currentPR == nil {
		for _, edge := range resp.Repository.PullRequests.Edges {
			if edge.Node.HeadLabel() == currentPRHeadRef {
				currentPR = &edge.Node
				break // Take the most recent PR for the current branch
			}
		}
	}

	payload := PullRequestsPayload{
		ViewerCreated: PullRequestAndTotalCount{
			PullRequests: viewerCreated,
			TotalCount:   resp.ViewerCreated.TotalCount,
		},
		ReviewRequested: PullRequestAndTotalCount{
			PullRequests: reviewRequested,
			TotalCount:   resp.ReviewRequested.TotalCount,
		},
		CurrentPR:     currentPR,
		DefaultBranch: resp.Repository.DefaultBranchRef.Name,
	}

	return &payload, nil
}

func pullRequestFragment(httpClient *http.Client, hostname string) (string, error) {
	cachedClient := NewCachedClient(httpClient, time.Hour*24)
	prFeatures, err := determinePullRequestFeatures(cachedClient, hostname)
	if err != nil {
		return "", err
	}

	var reviewsFragment string
	if prFeatures.HasReviewDecision {
		reviewsFragment = "reviewDecision"
	}

	var statusesFragment string
	if prFeatures.HasStatusCheckRollup {
		statusesFragment = `
		commits(last: 1) {
			nodes {
				commit {
					statusCheckRollup {
						contexts(last: 100) {
							nodes {
								...on StatusContext {
									state
								}
								...on CheckRun {
									conclusion
									status
								}
							}
						}
					}
				}
			}
		}
		`
	}

	var requiresStrictStatusChecks string
	if prFeatures.HasBranchProtectionRule {
		requiresStrictStatusChecks = `
		baseRef {
			branchProtectionRule {
				requiresStrictStatusChecks
			}
		}`
	}

	fragments := fmt.Sprintf(`
	fragment pr on PullRequest {
		number
		title
		state
		url
		headRefName
		mergeStateStatus
		headRepositoryOwner {
			login
		}
		%s
		isCrossRepository
		isDraft
		%s
	}
	fragment prWithReviews on PullRequest {
		...pr
		%s
	}
	`, requiresStrictStatusChecks, statusesFragment, reviewsFragment)
	return fragments, nil
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

func UpdatePullRequest(client *Client, repo ghrepo.Interface, params githubv4.UpdatePullRequestInput) error {
	var mutation struct {
		UpdatePullRequest struct {
			PullRequest struct {
				ID string
			}
		} `graphql:"updatePullRequest(input: $input)"`
	}
	variables := map[string]interface{}{"input": params}
	gql := graphQLClient(client.http, repo.RepoHost())
	err := gql.MutateNamed(context.Background(), "PullRequestUpdate", &mutation, variables)
	return err
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

func PullRequestClose(client *Client, repo ghrepo.Interface, pr *PullRequest) error {
	var mutation struct {
		ClosePullRequest struct {
			PullRequest struct {
				ID githubv4.ID
			}
		} `graphql:"closePullRequest(input: $input)"`
	}

	variables := map[string]interface{}{
		"input": githubv4.ClosePullRequestInput{
			PullRequestID: pr.ID,
		},
	}

	gql := graphQLClient(client.http, repo.RepoHost())
	err := gql.MutateNamed(context.Background(), "PullRequestClose", &mutation, variables)

	return err
}

func PullRequestReopen(client *Client, repo ghrepo.Interface, pr *PullRequest) error {
	var mutation struct {
		ReopenPullRequest struct {
			PullRequest struct {
				ID githubv4.ID
			}
		} `graphql:"reopenPullRequest(input: $input)"`
	}

	variables := map[string]interface{}{
		"input": githubv4.ReopenPullRequestInput{
			PullRequestID: pr.ID,
		},
	}

	gql := graphQLClient(client.http, repo.RepoHost())
	err := gql.MutateNamed(context.Background(), "PullRequestReopen", &mutation, variables)

	return err
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
