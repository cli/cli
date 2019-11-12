package api

import (
	"fmt"
	"time"
)

func IssueCreate(client *Client, ghRepo Repo, params map[string]interface{}) (*Issue, error) {
	repoID, err := GitHubRepoId(client, ghRepo)
	if err != nil {
		return nil, err
	}

	query := `
	mutation CreateIssue($input: CreateIssueInput!) {
		createIssue(input: $input) {
			issue {
				url
			}
		}
	}`

	inputParams := map[string]interface{}{
		"repositoryId": repoID,
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

	err = client.GraphQL(query, variables, &result)
	if err != nil {
		return nil, err
	}

	return &result.CreateIssue.Issue, nil
}

type IssuesPayload struct {
	Assigned  []Issue
	Mentioned []Issue
	Recent    []Issue
}

type Issue struct {
	Number int
	Title  string
	URL    string
}

type apiIssues struct {
	Issues struct {
		Edges []struct {
			Node Issue
		}
	}
}

func IssueStatus(client *Client, ghRepo Repo, currentUsername string) (*IssuesPayload, error) {
	type response struct {
		Assigned  apiIssues
		Mentioned apiIssues
		Recent    apiIssues
	}

	query := `
    fragment issue on Issue {
      number
      title
    }
    query($owner: String!, $repo: String!, $since: DateTime!, $viewer: String!, $per_page: Int = 10) {
      assigned: repository(owner: $owner, name: $repo) {
        issues(filterBy: {assignee: $viewer}, first: $per_page, orderBy: {field: CREATED_AT, direction: DESC}) {
          edges {
            node {
              ...issue
            }
          }
        }
      }
      mentioned: repository(owner: $owner, name: $repo) {
        issues(filterBy: {mentioned: $viewer}, first: $per_page, orderBy: {field: CREATED_AT, direction: DESC}) {
          edges {
            node {
              ...issue
            }
          }
        }
      }
      recent: repository(owner: $owner, name: $repo) {
        issues(filterBy: {since: $since}, first: $per_page, orderBy: {field: CREATED_AT, direction: DESC}) {
          edges {
            node {
              ...issue
            }
          }
        }
      }
    }
  `

	owner := ghRepo.RepoOwner()
	repo := ghRepo.RepoName()
	since := time.Now().UTC().Add(time.Hour * -24).Format("2006-01-02T15:04:05-0700")
	variables := map[string]interface{}{
		"owner":  owner,
		"repo":   repo,
		"viewer": currentUsername,
		"since":  since,
	}

	var resp response
	err := client.GraphQL(query, variables, &resp)
	if err != nil {
		return nil, err
	}

	var assigned []Issue
	for _, edge := range resp.Assigned.Issues.Edges {
		assigned = append(assigned, edge.Node)
	}

	var mentioned []Issue
	for _, edge := range resp.Mentioned.Issues.Edges {
		mentioned = append(mentioned, edge.Node)
	}

	var recent []Issue
	for _, edge := range resp.Recent.Issues.Edges {
		recent = append(recent, edge.Node)
	}

	payload := IssuesPayload{
		assigned,
		mentioned,
		recent,
	}

	return &payload, nil
}

func IssueList(client *Client, ghRepo Repo, state string) ([]Issue, error) {
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

	query := `
    fragment issue on Issue {
      number
      title
    }
    query($owner: String!, $repo: String!, $per_page: Int = 10, $states: [IssueState!] = OPEN) {
      repository(owner: $owner, name: $repo) {
        issues(first: $per_page, orderBy: {field: CREATED_AT, direction: DESC}, states: $states) {
          edges {
            node {
              ...issue
            }
          }
        }
      }
    }
  `

	owner := ghRepo.RepoOwner()
	repo := ghRepo.RepoName()
	variables := map[string]interface{}{
		"owner":  owner,
		"repo":   repo,
		"states": states,
	}

	var resp struct {
		Repository apiIssues
	}

	err := client.GraphQL(query, variables, &resp)
	if err != nil {
		return nil, err
	}

	var issues []Issue
	for _, edge := range resp.Repository.Issues.Edges {
		issues = append(issues, edge.Node)
	}

	return issues, nil
}
