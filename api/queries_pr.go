package api

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"
	"strings"

	"github.com/shurcooL/githubv4"
	"github.com/shurcooL/graphql"

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
	CurrentPR       *PullRequestComplex
	DefaultBranch   string
}

type PullRequestAndTotalCount struct {
	TotalCount   int
	PullRequests []PullRequestComplex
}

type LegacyPullRequestAndTotalCount struct {
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
	Mergeable   string

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
					} `graphql:"contexts(last: 100)"`
				}
			}
		}
	} `graphql:"commits(last: 1)"`

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

func (pr PullRequestComplex) HeadLabel() string {
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

func (pr *PullRequestComplex) ReviewStatus() PullRequestReviewStatus {
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

func (pr *PullRequestComplex) ChecksStatus() (summary PullRequestChecksStatus) {
	if len(pr.Commits.Nodes) == 0 {
		return
	}
	commit := pr.Commits.Nodes[0].Commit
	for _, c := range commit.StatusCheckRollup.Contexts.Nodes {
		state := c.StatusContext.State
		if state == "" {
			// CheckRun
			if c.CheckRun.Status == "COMPLETED" {
				state = c.CheckRun.Conclusion
			} else {
				state = c.CheckRun.Status
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

func (c Client) PullRequestDiff(baseRepo ghrepo.Interface, prNumber int) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/pulls/%d",
		ghrepo.FullName(baseRepo), prNumber)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Accept", "application/vnd.github.v3.diff; charset=utf-8")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode == 200 {
		return string(b), nil
	}

	if resp.StatusCode == 404 {
		return "", &NotFoundError{errors.New("pull request not found")}
	}

	return "", errors.New("pull request diff lookup failed")
}

func PullRequests(client *Client, repo ghrepo.Interface, currentPRNumber int, currentPRHeadRef, currentUsername string) (*PullRequestsPayload, error) {
	var query struct {
		ViewerCreated struct {
			TotalCount int `graphql:"issueCount"`
			Edges      []struct {
				Node struct {
					PullRequest PullRequestComplex `graphql:"...on PullRequest"`
				}
			}
		} `graphql:"viewerCreated: search(query: $viewerQuery, type: ISSUE, first: $perPage)"`

		ReviewRequested struct {
			TotalCount int `graphql:"issueCount"`
			Edges      []struct {
				Node struct {
					PullRequest PullRequestComplex `graphql:"...on PullRequest"`
				}
			}
		} `graphql:"reviewRequested: search(query: $reviewerQuery, type: ISSUE, first: $perPage)"`

		Repository struct {
			DefaultBranchRef struct{ Name string }
			PullRequests     struct {
				TotalCount int
				Edges      []struct {
					Node PullRequestComplex
				}
			} `graphql:"pullRequests(headRefName: $headRefName, first: $perPage, orderBy: { field: CREATED_AT, direction: DESC }) @skip(if: $singlePRSearch)"`
			PullRequest PullRequestComplex `graphql:"pullRequest(number: $number) @include(if: $singlePRSearch)"`
		} `graphql:"repository(owner: $owner, name: $repo)"`
	}

	viewerQuery := fmt.Sprintf("repo:%s state:open is:pr author:%s", ghrepo.FullName(repo), currentUsername)
	reviewerQuery := fmt.Sprintf("repo:%s state:open review-requested:%s", ghrepo.FullName(repo), currentUsername)

	branchWithoutOwner := currentPRHeadRef
	if idx := strings.Index(currentPRHeadRef, ":"); idx >= 0 {
		branchWithoutOwner = currentPRHeadRef[idx+1:]
	}

	variables := map[string]interface{}{
		"viewerQuery":    graphql.String(viewerQuery),
		"reviewerQuery":  graphql.String(reviewerQuery),
		"owner":          graphql.String(repo.RepoOwner()),
		"repo":           graphql.String(repo.RepoName()),
		"headRefName":    graphql.String(branchWithoutOwner),
		"number":         graphql.Int(currentPRNumber),
		"singlePRSearch": graphql.Boolean(currentPRNumber != 0),
		"perPage":        graphql.Int(10),
	}

	v4 := githubv4.NewClient(client.http)
	err := v4.Query(context.Background(), &query, variables)
	if err != nil {
		return nil, fmt.Errorf("failed to run graphql query to get pull requests: %s", err)
	}

	var viewerCreated []PullRequestComplex
	for _, edge := range query.ViewerCreated.Edges {
		viewerCreated = append(viewerCreated, edge.Node.PullRequest)
	}

	var reviewRequested []PullRequestComplex
	for _, edge := range query.ReviewRequested.Edges {
		reviewRequested = append(reviewRequested, edge.Node.PullRequest)
	}

	var currentPR *PullRequestComplex

	// Since query.Repository.PullRequest will never be nil, check instead for an empty ID
	if query.Repository.PullRequest.ID != "" {
		currentPR = &query.Repository.PullRequest
	} else {
		for _, edge := range query.Repository.PullRequests.Edges {
			if edge.Node.HeadLabel() == currentPRHeadRef {
				currentPR = &edge.Node
				break // Take the most recent PR for the current branch
			}
		}
	}

	payload := PullRequestsPayload{
		ViewerCreated: PullRequestAndTotalCount{
			PullRequests: viewerCreated,
			TotalCount:   query.ViewerCreated.TotalCount,
		},
		ReviewRequested: PullRequestAndTotalCount{
			PullRequests: reviewRequested,
			TotalCount:   query.ReviewRequested.TotalCount,
		},
		CurrentPR:     currentPR,
		DefaultBranch: query.Repository.DefaultBranchRef.Name,
	}

	return &payload, nil
}

type PullRequestMinimal struct {
	ID      string
	Number  int
	Title   string
	State   string
	Body    string
	URL     string
	IsDraft bool
	Closed  bool

	Author struct {
		Login string
	}

	BaseRefName         string
	HeadRefName         string
	IsCrossRepository   bool
	MaintainerCanModify bool
	HeadRepositoryOwner struct {
		Login string
	}
	HeadRepository struct {
		Name             string
		DefaultBranchRef struct {
			Name string
		}
	}
}

type PullRequestComplex struct {
	PullRequestMinimal

	ReviewDecision string

	Commits struct {
		TotalCount int
		Nodes      []struct {
			Commit struct {
				StatusCheckRollup struct {
					Contexts struct {
						Nodes []struct {
							StatusContext struct {
								State string
							} `graphql:"... on StatusContext"`
							CheckRun struct {
								Status     string
								Conclusion string
							} `graphql:"... on CheckRun"`
						}
					} `graphql:"contexts(last: 100)"`
				}
			}
		}
	} `graphql:"commits(last: 1)"`

	ReviewRequests struct {
		Nodes []struct {
			RequestedReviewer struct {
				Typename graphql.String `graphql:"__typename"`
				User     struct {
					Login string
				} `graphql:"... on User"`
				Team struct {
					Name string
				} `graphql:"... on Team"`
			}
		}
		TotalCount int
	} `graphql:"reviewRequests(first: 100)"`

	Assignees struct {
		Nodes []struct {
			Login string
		}
		TotalCount int
	} `graphql:"assignees(first: 100)"`

	Labels struct {
		Nodes []struct {
			Name string
		}
		TotalCount int
	} `graphql:"labels(first: 100)"`

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
	} `graphql:"projectCards(first: 100)"`

	Milestone struct {
		Title string
	}

	Reviews struct {
		Nodes []struct {
			Author struct {
				Login string
			}
			State string
		}
	} `graphql:"reviews(last: 100)"`
}

type PullRequestComplexWithMergable struct {
	PullRequestComplex

	Mergeable string
}

func PullRequestByNumber(client *Client, repo ghrepo.Interface, number int, pr interface{}) (bool, error) {
	query := reflect.New(reflect.StructOf([]reflect.StructField{
		{
			Name: "Repository",
			Tag:  `graphql:"repository(owner: $owner, name: $repo)"`,
			Type: reflect.StructOf([]reflect.StructField{
				{
					Name: "PullRequest",
					Tag:  `graphql:"pullRequest(number: $pr_number)"`,
					Type: reflect.TypeOf(pr),
				},
			}),
		},
	}))

	variables := map[string]interface{}{
		"owner":     githubv4.String(repo.RepoOwner()),
		"repo":      githubv4.String(repo.RepoName()),
		"pr_number": githubv4.Int(number),
	}

	v4 := githubv4.NewClient(client.http)
	err := v4.Query(context.Background(), query.Interface(), variables)
	if err != nil {
		return false, err
	}

	foundPr := query.Elem().FieldByName("Repository").FieldByName("PullRequest")
	reflect.Indirect(reflect.ValueOf(pr)).Set(reflect.Indirect(foundPr))

	return true, nil
}

func PullRequestForBranch(client *Client, repo ghrepo.Interface, baseBranch, headBranch string, pr interface{}) (bool, error) {
	query := reflect.New(reflect.StructOf([]reflect.StructField{
		{
			Name: "Repository",
			Tag:  `graphql:"repository(owner: $owner, name: $repo)"`,
			Type: reflect.StructOf([]reflect.StructField{
				{
					Name: "PullRequests",
					Tag:  `graphql:"pullRequests(headRefName: $headRefName, baseRefName: $baseRefName, states: OPEN, first: 30)"`,
					Type: reflect.StructOf([]reflect.StructField{
						{
							Name: "Nodes",
							Type: reflect.SliceOf(reflect.TypeOf(pr)),
						},
					}),
				},
			}),
		},
	}))

	branchWithoutOwner := headBranch
	if idx := strings.Index(headBranch, ":"); idx >= 0 {
		branchWithoutOwner = headBranch[idx+1:]
	}
	variables := map[string]interface{}{
		"owner":       githubv4.String(repo.RepoOwner()),
		"repo":        githubv4.String(repo.RepoName()),
		"headRefName": githubv4.String(branchWithoutOwner),
		"baseRefName": (*githubv4.String)(nil),
	}
	if baseBranch != "" {
		variables["baseRefName"] = githubv4.String(baseBranch)
	}

	v4 := githubv4.NewClient(client.http)
	err := v4.Query(context.Background(), query.Interface(), variables)
	if err != nil {
		return false, err
	}

	prsValue := query.Elem().FieldByName("Repository").FieldByName("PullRequests").FieldByName("Nodes")
	for i := 0; i < prsValue.Len(); i++ {
		foundPr := prsValue.Index(i).Elem()
		headLabel := ""
		owner := foundPr.FieldByName("HeadRepositoryOwner").FieldByName("Login").String()
		headRefName := foundPr.FieldByName("HeadRefName").String()
		if foundPr.FieldByName("IsCrossRepository").Bool() {
			headLabel = fmt.Sprintf("%s:%s", owner, headRefName)
		} else {
			headLabel = foundPr.FieldByName("HeadRefName").String()
		}

		if headLabel == headBranch {
			if baseBranch != "" {
				if foundPr.FieldByName("BaseRefName").String() != baseBranch {
					continue
				}
			}
			reflect.Indirect(reflect.ValueOf(pr)).Set(reflect.Indirect(foundPr))
			return true, nil
		}
	}

	return false, nil
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
		reviewParams["union"] = true
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

func AddReview(client *Client, pr PullRequestComplex, input *PullRequestReviewInput) error {
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

func PullRequestList(client *Client, vars map[string]interface{}, limit int) (*LegacyPullRequestAndTotalCount, error) {
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
	res := LegacyPullRequestAndTotalCount{}

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

func PullRequestClose(client *Client, repo ghrepo.Interface, pr PullRequestMinimal) error {
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

func PullRequestReopen(client *Client, repo ghrepo.Interface, pr PullRequestMinimal) error {
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

func PullRequestMerge(client *Client, repo ghrepo.Interface, pr PullRequestComplexWithMergable, m PullRequestMergeMethod) error {
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

func PullRequestReady(client *Client, repo ghrepo.Interface, pr PullRequestMinimal) error {
	var mutation struct {
		MarkPullRequestReadyForReviewInput struct {
			PullRequest struct {
				ID githubv4.ID
			}
		} `graphql:"markPullRequestReadyForReview(input: $input)"`
	}

	type MarkPullRequestReadyForReviewInput struct {
		PullRequestID githubv4.ID `json:"pullRequestId"`
	}

	input := MarkPullRequestReadyForReviewInput{PullRequestID: pr.ID}

	v4 := githubv4.NewClient(client.http)
	err := v4.Mutate(context.Background(), &mutation, input, nil)

	return err
}

func BranchDeleteRemote(client *Client, repo ghrepo.Interface, branch string) error {
	var response struct {
		NodeID string `json:"node_id"`
	}
	path := fmt.Sprintf("repos/%s/%s/git/refs/heads/%s", repo.RepoOwner(), repo.RepoName(), branch)
	return client.REST("DELETE", path, nil, &response)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
