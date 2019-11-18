package api

import (
	"fmt"
	"time"
)

type IssuesPayload struct {
	Assigned  []Issue
	Mentioned []Issue
	Recent    []Issue
}

type Issue struct {
	Number          int
	Title           string
	URL             string
	Labels          []string
	TotalLabelCount int
}

type apiIssues struct {
	Issues struct {
		Edges []struct {
			Node struct {
				Number int
				Title  string
				URL    string
				Labels struct {
					Edges []struct {
						Node struct {
							Name string
						}
					}
					TotalCount int
				}
			}
		}
	}
}

var fragments string

func init() {
	fragments = `
		fragment issue on Issue {
			number
			title
			labels(first: 3) {
				edges {
					node {
						name
					}
				}
				totalCount
			}
		}
	`
}

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

func IssueStatus(client *Client, ghRepo Repo, currentUsername string) (*IssuesPayload, error) {
	type response struct {
		Assigned  apiIssues
		Mentioned apiIssues
		Recent    apiIssues
	}

	query := fragments + `
    query($owner: String!, $repo: String!, $since: DateTime!, $viewer: String!, $per_page: Int = 10) {
      assigned: repository(owner: $owner, name: $repo) {
        issues(filterBy: {assignee: $viewer, states: OPEN}, first: $per_page, orderBy: {field: CREATED_AT, direction: DESC}) {
          edges {
            node {
              ...issue
            }
          }
        }
      }
      mentioned: repository(owner: $owner, name: $repo) {
        issues(filterBy: {mentioned: $viewer, states: OPEN}, first: $per_page, orderBy: {field: CREATED_AT, direction: DESC}) {
          edges {
            node {
              ...issue
            }
          }
        }
      }
      recent: repository(owner: $owner, name: $repo) {
        issues(filterBy: {since: $since, states: OPEN}, first: $per_page, orderBy: {field: CREATED_AT, direction: DESC}) {
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

	assigned := convertAPIToIssues(resp.Assigned)
	mentioned := convertAPIToIssues(resp.Mentioned)
	recent := convertAPIToIssues(resp.Recent)

	payload := IssuesPayload{
		assigned,
		mentioned,
		recent,
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

	// If you don't want to filter by lables, graphql requires you need
	// to send nil instead of an empty array.
	if len(labels) == 0 {
		labels = nil
	}

	var assignee interface{}
	if len(assigneeString) > 0 {
		assignee = assigneeString
	} else {
		assignee = nil
	}

	query := fragments + `
    query($owner: String!, $repo: String!, $limit: Int, $states: [IssueState!] = OPEN, $labels: [String!], $assignee: String) {
      repository(owner: $owner, name: $repo) {
        issues(first: $limit, orderBy: {field: CREATED_AT, direction: DESC}, states: $states, labels: $labels, filterBy: {assignee: $assignee}) {
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
		"limit":    limit,
		"owner":    owner,
		"repo":     repo,
		"states":   states,
		"labels":   labels,
		"assignee": assignee,
	}

	var resp struct {
		Repository apiIssues
	}

	err := client.GraphQL(query, variables, &resp)
	if err != nil {
		return nil, err
	}

	issues := convertAPIToIssues(resp.Repository)
	return issues, nil
}

func convertAPIToIssues(i apiIssues) []Issue {
	var issues []Issue
	for _, edge := range i.Issues.Edges {
		var labels []string
		for _, labelEdge := range edge.Node.Labels.Edges {
			labels = append(labels, labelEdge.Node.Name)
		}

		issue := Issue{
			Number:          edge.Node.Number,
			Title:           edge.Node.Title,
			URL:             edge.Node.URL,
			Labels:          labels,
			TotalLabelCount: edge.Node.Labels.TotalCount,
		}
		issues = append(issues, issue)
	}

	return issues
}
