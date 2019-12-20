package api

import (
	"fmt"
)

type IssuesPayload struct {
	Assigned  []Issue
	Mentioned []Issue
	Authored  []Issue
}

type Issue struct {
	Number int
	Title  string
	URL    string
	State  string

	Labels struct {
		Nodes      []IssueLabel
		TotalCount int
	}
}

type IssueLabel struct {
	Name string
}

const fragments = `
	fragment issue on Issue {
		number
		title
		url
		state
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

func IssueStatus(client *Client, ghRepo Repo, currentUsername string) (*IssuesPayload, error) {
	type response struct {
		Repository struct {
			Assigned struct {
				Nodes []Issue
			}
			Mentioned struct {
				Nodes []Issue
			}
			Authored struct {
				Nodes []Issue
			}
			HasIssuesEnabled bool
		}
	}

	query := fragments + `
	query($owner: String!, $repo: String!, $viewer: String!, $per_page: Int = 10) {
		repository(owner: $owner, name: $repo) {
			hasIssuesEnabled
			assigned: issues(filterBy: {assignee: $viewer, states: OPEN}, first: $per_page, orderBy: {field: CREATED_AT, direction: DESC}) {
				nodes {
					...issue
				}
			}
			mentioned: issues(filterBy: {mentioned: $viewer, states: OPEN}, first: $per_page, orderBy: {field: CREATED_AT, direction: DESC}) {
				nodes {
					...issue
				}
			}
			authored: issues(filterBy: {createdBy: $viewer, states: OPEN}, first: $per_page, orderBy: {field: CREATED_AT, direction: DESC}) {
				nodes {
					...issue
				}
			}
		}
    }`

	owner := ghRepo.RepoOwner()
	repo := ghRepo.RepoName()
	variables := map[string]interface{}{
		"owner":  owner,
		"repo":   repo,
		"viewer": currentUsername,
	}

	var resp response
	err := client.GraphQL(query, variables, &resp)
	if err != nil {
		return nil, err
	}

	if !resp.Repository.HasIssuesEnabled {
		return nil, fmt.Errorf("the '%s/%s' repository has disabled issues", owner, repo)
	}

	payload := IssuesPayload{
		Assigned:  resp.Repository.Assigned.Nodes,
		Mentioned: resp.Repository.Mentioned.Nodes,
		Authored:  resp.Repository.Authored.Nodes,
	}

	return &payload, nil
}

func IssueList(client *Client, ghRepo Repo, state string, labels []string, assigneeString string, limit int) ([]Issue, error) {
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
    query($owner: String!, $repo: String!, $limit: Int, $states: [IssueState!] = OPEN, $labels: [String!], $assignee: String) {
      repository(owner: $owner, name: $repo) {
		hasIssuesEnabled
        issues(first: $limit, orderBy: {field: CREATED_AT, direction: DESC}, states: $states, labels: $labels, filterBy: {assignee: $assignee}) {
          nodes {
            ...issue
          }
        }
      }
    }
  `

	owner := ghRepo.RepoOwner()
	repo := ghRepo.RepoName()
	variables := map[string]interface{}{
		"limit":  limit,
		"owner":  owner,
		"repo":   repo,
		"states": states,
	}
	if len(labels) > 0 {
		variables["labels"] = labels
	}
	if assigneeString != "" {
		variables["assignee"] = assigneeString
	}

	var resp struct {
		Repository struct {
			Issues struct {
				Nodes []Issue
			}
			HasIssuesEnabled bool
		}
	}

	err := client.GraphQL(query, variables, &resp)
	if err != nil {
		return nil, err
	}

	if !resp.Repository.HasIssuesEnabled {
		return nil, fmt.Errorf("the '%s/%s' repository has disabled issues", owner, repo)
	}

	return resp.Repository.Issues.Nodes, nil
}

func IssueByNumber(client *Client, ghRepo Repo, number int) (*Issue, error) {
	type response struct {
		Repository struct {
			Issue Issue
		}
	}

	query := `
	query($owner: String!, $repo: String!, $issue_number: Int!) {
		repository(owner: $owner, name: $repo) {
			issue(number: $issue_number) {
				number
				url
			}
		}
	}`

	variables := map[string]interface{}{
		"owner":        ghRepo.RepoOwner(),
		"repo":         ghRepo.RepoName(),
		"issue_number": number,
	}

	var resp response
	err := client.GraphQL(query, variables, &resp)
	if err != nil {
		return nil, err
	}

	return &resp.Repository.Issue, nil
}
