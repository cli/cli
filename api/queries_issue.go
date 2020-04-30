package api

import (
	"context"
	"fmt"
	"time"

	"github.com/cli/cli/internal/ghrepo"
	"github.com/shurcooL/githubv4"
)

type IssuesPayload struct {
	Assigned  IssuesAndTotalCount
	Mentioned IssuesAndTotalCount
	Authored  IssuesAndTotalCount
}

type IssuesAndTotalCount struct {
	Issues     []Issue
	TotalCount int
}

// Ref. https://developer.github.com/v4/object/issue/
type Issue struct {
	ID        string
	Number    int
	Title     string
	URL       string
	State     string
	Closed    bool
	Body      string
	CreatedAt time.Time
	UpdatedAt time.Time
	Comments  struct {
		TotalCount int
	}
	Author struct {
		Login string
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

type IssuesDisabledError struct {
	error
}

const fragments = `
	fragment issue on Issue {
		number
		title
		url
		state
		updatedAt
		labels(first: 3) {
			nodes {
				name
			}
			totalCount
		}
	}
`

// IssueCreate creates an issue in a GitHub repository
func IssueCreate(client *Client, repo *Repository, params map[string]interface{}) (*Issue, error) {
	query := `
	mutation CreateIssue($input: CreateIssueInput!) {
		createIssue(input: $input) {
			issue {
				url
			}
		}
	}`

	inputParams := map[string]interface{}{
		"repositoryId": repo.ID,
	}
	for key, val := range params {
		inputParams[key] = val
	}
	variables := map[string]interface{}{
		"input": inputParams,
	}

	result := struct {
		CreateIssue struct {
			Issue Issue
		}
	}{}

	err := client.GraphQL(query, variables, &result)
	if err != nil {
		return nil, err
	}

	return &result.CreateIssue.Issue, nil
}

func IssueStatus(client *Client, repo ghrepo.Interface, currentUsername string) (*IssuesPayload, error) {
	type response struct {
		Repository struct {
			Assigned struct {
				TotalCount int
				Nodes      []Issue
			}
			Mentioned struct {
				TotalCount int
				Nodes      []Issue
			}
			Authored struct {
				TotalCount int
				Nodes      []Issue
			}
			HasIssuesEnabled bool
		}
	}

	query := fragments + `
	query($owner: String!, $repo: String!, $viewer: String!, $per_page: Int = 10) {
		repository(owner: $owner, name: $repo) {
			hasIssuesEnabled
			assigned: issues(filterBy: {assignee: $viewer, states: OPEN}, first: $per_page, orderBy: {field: UPDATED_AT, direction: DESC}) {
				totalCount
				nodes {
					...issue
				}
			}
			mentioned: issues(filterBy: {mentioned: $viewer, states: OPEN}, first: $per_page, orderBy: {field: UPDATED_AT, direction: DESC}) {
				totalCount
				nodes {
					...issue
				}
			}
			authored: issues(filterBy: {createdBy: $viewer, states: OPEN}, first: $per_page, orderBy: {field: UPDATED_AT, direction: DESC}) {
				totalCount
				nodes {
					...issue
				}
			}
		}
    }`

	variables := map[string]interface{}{
		"owner":  repo.RepoOwner(),
		"repo":   repo.RepoName(),
		"viewer": currentUsername,
	}

	var resp response
	err := client.GraphQL(query, variables, &resp)
	if err != nil {
		return nil, err
	}

	if !resp.Repository.HasIssuesEnabled {
		return nil, fmt.Errorf("the '%s' repository has disabled issues", ghrepo.FullName(repo))
	}

	payload := IssuesPayload{
		Assigned: IssuesAndTotalCount{
			Issues:     resp.Repository.Assigned.Nodes,
			TotalCount: resp.Repository.Assigned.TotalCount,
		},
		Mentioned: IssuesAndTotalCount{
			Issues:     resp.Repository.Mentioned.Nodes,
			TotalCount: resp.Repository.Mentioned.TotalCount,
		},
		Authored: IssuesAndTotalCount{
			Issues:     resp.Repository.Authored.Nodes,
			TotalCount: resp.Repository.Authored.TotalCount,
		},
	}

	return &payload, nil
}

func IssueList(client *Client, repo ghrepo.Interface, state string, labels []string, assigneeString string, limit int, authorString string) (*IssuesAndTotalCount, error) {
	var states []string
	switch state {
	case "open", "":
		states = []string{"OPEN"}
	case "closed":
		states = []string{"CLOSED"}
	case "all":
		states = []string{"OPEN", "CLOSED"}
	default:
		return nil, fmt.Errorf("invalid state: %s", state)
	}

	query := fragments + `
	query($owner: String!, $repo: String!, $limit: Int, $endCursor: String, $states: [IssueState!] = OPEN, $labels: [String!], $assignee: String, $author: String) {
		repository(owner: $owner, name: $repo) {
			hasIssuesEnabled
			issues(first: $limit, after: $endCursor, orderBy: {field: CREATED_AT, direction: DESC}, states: $states, labels: $labels, filterBy: {assignee: $assignee, createdBy: $author}) {
				totalCount
				nodes {
					...issue
				}
				pageInfo {
					hasNextPage
					endCursor
				}
			}
		}
	}
	`

	variables := map[string]interface{}{
		"owner":  repo.RepoOwner(),
		"repo":   repo.RepoName(),
		"states": states,
	}
	if len(labels) > 0 {
		variables["labels"] = labels
	}
	if assigneeString != "" {
		variables["assignee"] = assigneeString
	}
	if authorString != "" {
		variables["author"] = authorString
	}

	var response struct {
		Repository struct {
			Issues struct {
				TotalCount int
				Nodes      []Issue
				PageInfo   struct {
					HasNextPage bool
					EndCursor   string
				}
			}
			HasIssuesEnabled bool
		}
	}

	var issues []Issue
	pageLimit := min(limit, 100)

loop:
	for {
		variables["limit"] = pageLimit
		err := client.GraphQL(query, variables, &response)
		if err != nil {
			return nil, err
		}
		if !response.Repository.HasIssuesEnabled {
			return nil, fmt.Errorf("the '%s' repository has disabled issues", ghrepo.FullName(repo))
		}

		for _, issue := range response.Repository.Issues.Nodes {
			issues = append(issues, issue)
			if len(issues) == limit {
				break loop
			}
		}

		if response.Repository.Issues.PageInfo.HasNextPage {
			variables["endCursor"] = response.Repository.Issues.PageInfo.EndCursor
			pageLimit = min(pageLimit, limit-len(issues))
		} else {
			break
		}
	}

	res := IssuesAndTotalCount{Issues: issues, TotalCount: response.Repository.Issues.TotalCount}
	return &res, nil
}

func IssueByNumber(client *Client, repo ghrepo.Interface, number int) (*Issue, error) {
	type response struct {
		Repository struct {
			Issue            Issue
			HasIssuesEnabled bool
		}
	}

	query := `
	query($owner: String!, $repo: String!, $issue_number: Int!) {
		repository(owner: $owner, name: $repo) {
			hasIssuesEnabled
			issue(number: $issue_number) {
				id
				title
				state
				closed
				body
				author {
					login
				}
				comments {
					totalCount
				}
				number
				url
				createdAt
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
		"owner":        repo.RepoOwner(),
		"repo":         repo.RepoName(),
		"issue_number": number,
	}

	var resp response
	err := client.GraphQL(query, variables, &resp)
	if err != nil {
		return nil, err
	}

	if !resp.Repository.HasIssuesEnabled {

		return nil, &IssuesDisabledError{fmt.Errorf("the '%s' repository has disabled issues", ghrepo.FullName(repo))}
	}

	return &resp.Repository.Issue, nil
}

func IssueClose(client *Client, repo ghrepo.Interface, issue Issue) error {
	var mutation struct {
		CloseIssue struct {
			Issue struct {
				ID githubv4.ID
			}
		} `graphql:"closeIssue(input: $input)"`
	}

	input := githubv4.CloseIssueInput{
		IssueID: issue.ID,
	}

	v4 := githubv4.NewClient(client.http)
	err := v4.Mutate(context.Background(), &mutation, input, nil)

	if err != nil {
		return err
	}

	return nil
}

func IssueReopen(client *Client, repo ghrepo.Interface, issue Issue) error {
	var mutation struct {
		ReopenIssue struct {
			Issue struct {
				ID githubv4.ID
			}
		} `graphql:"reopenIssue(input: $input)"`
	}

	input := githubv4.ReopenIssueInput{
		IssueID: issue.ID,
	}

	v4 := githubv4.NewClient(client.http)
	err := v4.Mutate(context.Background(), &mutation, input, nil)

	return err
}
