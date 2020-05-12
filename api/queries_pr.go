package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/shurcooL/githubv4"

	"github.com/cli/cli/internal/ghrepo"
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
	ID          string
	Number      int
	Title       string
	State       string
	Closed      bool
	URL         string
	BaseRefName string
	HeadRefName string
	Body        string

	Author struct {
		Login string
	}
	HeadRepositoryOwner struct {
		Login string
	}
	HeadRepository struct {
		Name             string
		DefaultBranchRef struct {
			Name string
		}
	}
	IsCrossRepository   bool
	IsDraft             bool
	MaintainerCanModify bool

	ReviewDecision string

	Commits struct {
		TotalCount int
		Nodes      []struct {
			Commit struct {
				StatusCheckRollup struct {
					Contexts struct {
						Nodes []struct {
							State      string
							Status     string
							Conclusion string
						}
					}
				}
			}
		}
	}
	ReviewRequests struct {
		Nodes []struct {
			RequestedReviewer struct {
				TypeName string `json:"__typename"`
				Login    string
				Name     string
			}
		}
		TotalCount int
	}
	Reviews struct {
		Nodes []struct {
			Author struct {
				Login string
			}
			State string
		}
	}
	Assignees struct {
		Nodes []struct {
			Login string
		}
		TotalCount int
	}
	Labels struct {
		Nodes []struct {
			Name string
		}
		TotalCount int
	}
	ProjectCards struct {
		Nodes []struct {
			Project struct {
				Name string
			}
			Column struct {
				Name string
			}
		}
		TotalCount int
	}
	Milestone struct {
		Title string
	}
}

type NotFoundError struct {
	error
}

func (pr PullRequest) HeadLabel() string {
	if pr.IsCrossRepository {
		return fmt.Sprintf("%s:%s", pr.HeadRepositoryOwner.Login, pr.HeadRefName)
	}
	return pr.HeadRefName
}

type PullRequestReviewStatus struct {
	ChangesRequested bool
	Approved         bool
	ReviewRequired   bool
}

type PullRequestMergeMethod int

const (
	PullRequestMergeMethodMerge PullRequestMergeMethod = iota
	PullRequestMergeMethodRebase
	PullRequestMergeMethodSquash
)

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
	if len(pr.Commits.Nodes) == 0 {
		return
	}
	commit := pr.Commits.Nodes[0].Commit
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
		case "EXPECTED", "REQUESTED", "QUEUED", "PENDING", "IN_PROGRESS", "STALE":
			summary.Pending++
		default:
			panic(fmt.Errorf("unsupported status: %q", state))
		}
		summary.Total++
	}
	return
}

func PullRequests(client *Client, repo ghrepo.Interface, currentPRNumber int, currentPRHeadRef, currentUsername string) (*PullRequestsPayload, error) {
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

	fragments := `
	fragment pr on PullRequest {
		number
		title
		state
		url
		headRefName
		headRepositoryOwner {
			login
		}
		isCrossRepository
		isDraft
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
									status
									conclusion
								}
							}
						}
					}
				}
			}
		}
	}
	fragment prWithReviews on PullRequest {
		...pr
		reviewDecision
	}
	`

	queryPrefix := `
	query($owner: String!, $repo: String!, $headRefName: String!, $viewerQuery: String!, $reviewerQuery: String!, $per_page: Int = 10) {
		repository(owner: $owner, name: $repo) {
			defaultBranchRef { name }
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
	if currentPRNumber > 0 {
		queryPrefix = `
		query($owner: String!, $repo: String!, $number: Int!, $viewerQuery: String!, $reviewerQuery: String!, $per_page: Int = 10) {
			repository(owner: $owner, name: $repo) {
				defaultBranchRef { name }
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

	viewerQuery := fmt.Sprintf("repo:%s state:open is:pr author:%s", ghrepo.FullName(repo), currentUsername)
	reviewerQuery := fmt.Sprintf("repo:%s state:open review-requested:%s", ghrepo.FullName(repo), currentUsername)

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
		"number":        currentPRNumber,
	}

	var resp response
	err := client.GraphQL(query, variables, &resp)
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

func PullRequestByNumber(client *Client, repo ghrepo.Interface, number int) (*PullRequest, error) {
	type response struct {
		Repository struct {
			PullRequest PullRequest
		}
	}

	query := `
	query($owner: String!, $repo: String!, $pr_number: Int!) {
		repository(owner: $owner, name: $repo) {
			pullRequest(number: $pr_number) {
				id
				url
				number
				title
				state
				closed
				body
				author {
				  login
				}
				commits {
				  totalCount
				}
				baseRefName
				headRefName
				headRepositoryOwner {
					login
				}
				headRepository {
					name
					defaultBranchRef {
						name
					}
				}
				isCrossRepository
				isDraft
				maintainerCanModify
				reviewRequests(first: 100) {
					nodes {
						requestedReviewer {
							__typename
							...on User {
								login
							}
							...on Team {
								name
							}
						}
					}
					totalCount
				}
				reviews(last: 100) {
					nodes {
						author {
						  login
						}
						state
					}
					totalCount
				}
				assignees(first: 100) {
					nodes {
						login
					}
					totalCount
				}
				labels(first: 100) {
					nodes {
						name
					}
					totalCount
				}
				projectCards(first: 100) {
					nodes {
						project {
							name
						}
						column {
							name
						}
					}
					totalCount
				}
				milestone{
					title
				}
			}
		}
	}`

	variables := map[string]interface{}{
		"owner":     repo.RepoOwner(),
		"repo":      repo.RepoName(),
		"pr_number": number,
	}

	var resp response
	err := client.GraphQL(query, variables, &resp)
	if err != nil {
		return nil, err
	}

	return &resp.Repository.PullRequest, nil
}

func PullRequestForBranch(client *Client, repo ghrepo.Interface, baseBranch, headBranch string) (*PullRequest, error) {
	type response struct {
		Repository struct {
			PullRequests struct {
				ID    githubv4.ID
				Nodes []PullRequest
			}
		}
	}

	query := `
	query($owner: String!, $repo: String!, $headRefName: String!) {
		repository(owner: $owner, name: $repo) {
			pullRequests(headRefName: $headRefName, states: OPEN, first: 30) {
				nodes {
					id
					number
					title
					state
					body
					author {
						login
					}
					commits {
						totalCount
					}
					url
					baseRefName
					headRefName
					headRepositoryOwner {
						login
					}
					isCrossRepository
					isDraft
					reviewRequests(first: 100) {
						nodes {
							requestedReviewer {
								__typename
								...on User {
									login
								}
								...on Team {
									name
								}
							}
						}
						totalCount
					}
					reviews(last: 100) {
						nodes {
							author {
							  login
							}
							state
						}
						totalCount
					}
					assignees(first: 100) {
						nodes {
							login
						}
						totalCount
					}
					labels(first: 100) {
						nodes {
							name
						}
						totalCount
					}
					projectCards(first: 100) {
						nodes {
							project {
								name
							}
							column {
								name
							}
						}
						totalCount
					}
					milestone{
						title
					}
				}
			}
		}
	}`

	branchWithoutOwner := headBranch
	if idx := strings.Index(headBranch, ":"); idx >= 0 {
		branchWithoutOwner = headBranch[idx+1:]
	}

	variables := map[string]interface{}{
		"owner":       repo.RepoOwner(),
		"repo":        repo.RepoName(),
		"headRefName": branchWithoutOwner,
	}

	var resp response
	err := client.GraphQL(query, variables, &resp)
	if err != nil {
		return nil, err
	}

	for _, pr := range resp.Repository.PullRequests.Nodes {
		if pr.HeadLabel() == headBranch {
			if baseBranch != "" {
				if pr.BaseRefName != baseBranch {
					continue
				}
			}
			return &pr, nil
		}
	}

	return nil, &NotFoundError{fmt.Errorf("no open pull requests found for branch %q", headBranch)}
}

// CreatePullRequest creates a pull request in a GitHub repository
func CreatePullRequest(client *Client, repo *Repository, params map[string]interface{}) (*PullRequest, error) {
	query := `
		mutation CreatePullRequest($input: CreatePullRequestInput!) {
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
		case "title", "body", "draft", "baseRefName", "headRefName":
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

	err := client.GraphQL(query, variables, &result)
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
		mutation UpdatePullRequest($input: UpdatePullRequestInput!) {
			updatePullRequest(input: $input) { clientMutationId }
		}`
		updateParams["pullRequestId"] = pr.ID
		variables := map[string]interface{}{
			"input": updateParams,
		}
		err := client.GraphQL(updateQuery, variables, &result)
		if err != nil {
			return nil, err
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

	if len(reviewParams) > 0 {
		reviewQuery := `
		mutation RequestReviews($input: RequestReviewsInput!) {
			requestReviews(input: $input) { clientMutationId }
		}`
		reviewParams["pullRequestId"] = pr.ID
		variables := map[string]interface{}{
			"input": reviewParams,
		}
		err := client.GraphQL(reviewQuery, variables, &result)
		if err != nil {
			return nil, err
		}
	}

	return pr, nil
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

func AddReview(client *Client, pr *PullRequest, input *PullRequestReviewInput) error {
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

	gqlInput := githubv4.AddPullRequestReviewInput{
		PullRequestID: pr.ID,
		Event:         &state,
		Body:          &body,
	}

	v4 := githubv4.NewClient(client.http)

	return v4.Mutate(context.Background(), &mutation, gqlInput, nil)
}

func PullRequestList(client *Client, vars map[string]interface{}, limit int) (*PullRequestAndTotalCount, error) {
	type prBlock struct {
		Edges []struct {
			Node PullRequest
		}
		PageInfo struct {
			HasNextPage bool
			EndCursor   string
		}
		TotalCount int
		IssueCount int
	}
	type response struct {
		Repository struct {
			PullRequests prBlock
		}
		Search prBlock
	}

	fragment := `
	fragment pr on PullRequest {
		number
		title
		state
		url
		headRefName
		headRepositoryOwner {
			login
		}
		isCrossRepository
		isDraft
	}
	`

	// If assignee wasn't specified, use `Repository.pullRequest` for ability to
	// query by multiple labels
	query := fragment + `
    query(
		$owner: String!,
		$repo: String!,
		$limit: Int!,
		$endCursor: String,
		$baseBranch: String,
		$labels: [String!],
		$state: [PullRequestState!] = OPEN
	) {
      repository(owner: $owner, name: $repo) {
        pullRequests(
			states: $state,
			baseRefName: $baseBranch,
			labels: $labels,
			first: $limit,
			after: $endCursor,
			orderBy: {field: CREATED_AT, direction: DESC}
			) {
				totalCount
				edges {
					node {
						...pr
					}
				}
				pageInfo {
					hasNextPage
					endCursor
				}
			}
		}
	}`

	var check = make(map[int]struct{})
	var prs []PullRequest
	pageLimit := min(limit, 100)
	variables := map[string]interface{}{}
	res := PullRequestAndTotalCount{}

	// If assignee was specified, use the `search` API rather than
	// `Repository.pullRequests`, but this mode doesn't support multiple labels
	if assignee, ok := vars["assignee"].(string); ok {
		query = fragment + `
		query(
			$q: String!,
			$limit: Int!,
			$endCursor: String,
		) {
			search(query: $q, type: ISSUE, first: $limit, after: $endCursor) {
				issueCount
				edges {
					node {
						...pr
					}
				}
				pageInfo {
					hasNextPage
					endCursor
				}
			}
		}`
		owner := vars["owner"].(string)
		repo := vars["repo"].(string)
		search := []string{
			fmt.Sprintf("repo:%s/%s", owner, repo),
			fmt.Sprintf("assignee:%s", assignee),
			"is:pr",
			"sort:created-desc",
		}
		if states, ok := vars["state"].([]string); ok && len(states) == 1 {
			switch states[0] {
			case "OPEN":
				search = append(search, "state:open")
			case "CLOSED":
				search = append(search, "state:closed")
			case "MERGED":
				search = append(search, "is:merged")
			}
		}
		if labels, ok := vars["labels"].([]string); ok && len(labels) > 0 {
			if len(labels) > 1 {
				return nil, fmt.Errorf("multiple labels with --assignee are not supported")
			}
			search = append(search, fmt.Sprintf(`label:"%s"`, labels[0]))
		}
		if baseBranch, ok := vars["baseBranch"].(string); ok {
			search = append(search, fmt.Sprintf(`base:"%s"`, baseBranch))
		}
		variables["q"] = strings.Join(search, " ")
	} else {
		for name, val := range vars {
			variables[name] = val
		}
	}
loop:
	for {
		variables["limit"] = pageLimit
		var data response
		err := client.GraphQL(query, variables, &data)
		if err != nil {
			return nil, err
		}
		prData := data.Repository.PullRequests
		res.TotalCount = prData.TotalCount
		if _, ok := variables["q"]; ok {
			prData = data.Search
			res.TotalCount = prData.IssueCount
		}

		for _, edge := range prData.Edges {
			if _, exists := check[edge.Node.Number]; exists {
				continue
			}

			prs = append(prs, edge.Node)
			check[edge.Node.Number] = struct{}{}
			if len(prs) == limit {
				break loop
			}
		}

		if prData.PageInfo.HasNextPage {
			variables["endCursor"] = prData.PageInfo.EndCursor
			pageLimit = min(pageLimit, limit-len(prs))
		} else {
			break
		}
	}
	res.PullRequests = prs
	return &res, nil
}

func PullRequestClose(client *Client, repo ghrepo.Interface, pr *PullRequest) error {
	var mutation struct {
		ClosePullRequest struct {
			PullRequest struct {
				ID githubv4.ID
			}
		} `graphql:"closePullRequest(input: $input)"`
	}

	input := githubv4.ClosePullRequestInput{
		PullRequestID: pr.ID,
	}

	v4 := githubv4.NewClient(client.http)
	err := v4.Mutate(context.Background(), &mutation, input, nil)

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

	input := githubv4.ReopenPullRequestInput{
		PullRequestID: pr.ID,
	}

	v4 := githubv4.NewClient(client.http)
	err := v4.Mutate(context.Background(), &mutation, input, nil)

	return err
}

func PullRequestMerge(client *Client, repo ghrepo.Interface, pr *PullRequest, m PullRequestMergeMethod) error {
	mergeMethod := githubv4.PullRequestMergeMethodMerge
	switch m {
	case PullRequestMergeMethodRebase:
		mergeMethod = githubv4.PullRequestMergeMethodRebase
	case PullRequestMergeMethodSquash:
		mergeMethod = githubv4.PullRequestMergeMethodSquash
	}

	var mutation struct {
		MergePullRequest struct {
			PullRequest struct {
				ID githubv4.ID
			}
		} `graphql:"mergePullRequest(input: $input)"`
	}

	input := githubv4.MergePullRequestInput{
		PullRequestID: pr.ID,
		MergeMethod:   &mergeMethod,
	}

	v4 := githubv4.NewClient(client.http)
	err := v4.Mutate(context.Background(), &mutation, input, nil)

	return err
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
