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

type Issue struct {
	ID             string
	Number         int
	Title          string
	URL            string
	State          string
	Closed         bool
	Body           string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	ClosedAt       *time.Time
	Comments       Comments
	Author         Author
	Assignees      Assignees
	Labels         Labels
	ProjectCards   ProjectCards
	Milestone      *Milestone
	ReactionGroups ReactionGroups
}

type Assignees struct {
	Nodes      []GitHubUser
	TotalCount int
}

func (a Assignees) Logins() []string {
	logins := make([]string, len(a.Nodes))
	for i, a := range a.Nodes {
		logins[i] = a.Login
	}
	return logins
}

type Labels struct {
	Nodes      []IssueLabel
	TotalCount int
}

func (l Labels) Names() []string {
	names := make([]string, len(l.Nodes))
	for i, l := range l.Nodes {
		names[i] = l.Name
	}
	return names
}

type ProjectCards struct {
	Nodes []struct {
		Project struct {
			Name string `json:"name"`
		} `json:"project"`
		Column struct {
			Name string `json:"name"`
		} `json:"column"`
	}
	TotalCount int
}

func (p ProjectCards) ProjectNames() []string {
	names := make([]string, len(p.Nodes))
	for i, c := range p.Nodes {
		names[i] = c.Project.Name
	}
	return names
}

type Milestone struct {
	Number      int        `json:"number"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	DueOn       *time.Time `json:"dueOn"`
}

type IssuesDisabledError struct {
	error
}

type Owner struct {
	ID    string `json:"id,omitempty"`
	Name  string `json:"name,omitempty"`
	Login string `json:"login"`
}

type Author struct {
	// adding these breaks generated GraphQL requests
	//ID    string `json:"id,omitempty"`
	//Name  string `json:"name,omitempty"`
	Login string `json:"login"`
}

// IssueCreate creates an issue in a GitHub repository
func IssueCreate(client *Client, repo *Repository, params map[string]interface{}) (*Issue, error) {
	query := `
	mutation IssueCreate($input: CreateIssueInput!) {
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

	err := client.GraphQL(repo.RepoHost(), query, variables, &result)
	if err != nil {
		return nil, err
	}

	return &result.CreateIssue.Issue, nil
}

type IssueStatusOptions struct {
	Username string
	Fields   []string
}

func IssueStatus(client *Client, repo ghrepo.Interface, options IssueStatusOptions) (*IssuesPayload, error) {
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

	fragments := fmt.Sprintf("fragment issue on Issue{%s}", PullRequestGraphQL(options.Fields))
	query := fragments + `
	query IssueStatus($owner: String!, $repo: String!, $viewer: String!, $per_page: Int = 10) {
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
		"viewer": options.Username,
	}

	var resp response
	err := client.GraphQL(repo.RepoHost(), query, variables, &resp)
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

func IssueByNumber(client *Client, repo ghrepo.Interface, number int) (*Issue, error) {
	type response struct {
		Repository struct {
			Issue            Issue
			HasIssuesEnabled bool
		}
	}

	query := `
	query IssueByNumber($owner: String!, $repo: String!, $issue_number: Int!) {
		repository(owner: $owner, name: $repo) {
			hasIssuesEnabled
			issue(number: $issue_number) {
				id
				title
				state
				body
				author {
					login
				}
				comments(last: 1) {
					nodes {
						author {
							login
						}
						authorAssociation
						body
						createdAt
						includesCreatedEdit
						isMinimized
						minimizedReason
						reactionGroups {
							content
							users {
								totalCount
							}
						}
					}
					totalCount
				}
				number
				url
				createdAt
				assignees(first: 100) {
					nodes {
						id
						name
						login
					}
					totalCount
				}
				labels(first: 100) {
					nodes {
						id
						name
						description
						color
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
				milestone {
					number
					title
					description
					dueOn
				}
				reactionGroups {
					content
					users {
						totalCount
					}
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
	err := client.GraphQL(repo.RepoHost(), query, variables, &resp)
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

	variables := map[string]interface{}{
		"input": githubv4.CloseIssueInput{
			IssueID: issue.ID,
		},
	}

	gql := graphQLClient(client.http, repo.RepoHost())
	err := gql.MutateNamed(context.Background(), "IssueClose", &mutation, variables)

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

	variables := map[string]interface{}{
		"input": githubv4.ReopenIssueInput{
			IssueID: issue.ID,
		},
	}

	gql := graphQLClient(client.http, repo.RepoHost())
	err := gql.MutateNamed(context.Background(), "IssueReopen", &mutation, variables)

	return err
}

func IssueDelete(client *Client, repo ghrepo.Interface, issue Issue) error {
	var mutation struct {
		DeleteIssue struct {
			Repository struct {
				ID githubv4.ID
			}
		} `graphql:"deleteIssue(input: $input)"`
	}

	variables := map[string]interface{}{
		"input": githubv4.DeleteIssueInput{
			IssueID: issue.ID,
		},
	}

	gql := graphQLClient(client.http, repo.RepoHost())
	err := gql.MutateNamed(context.Background(), "IssueDelete", &mutation, variables)

	return err
}

func IssueUpdate(client *Client, repo ghrepo.Interface, params githubv4.UpdateIssueInput) error {
	var mutation struct {
		UpdateIssue struct {
			Issue struct {
				ID string
			}
		} `graphql:"updateIssue(input: $input)"`
	}
	variables := map[string]interface{}{"input": params}
	gql := graphQLClient(client.http, repo.RepoHost())
	err := gql.MutateNamed(context.Background(), "IssueUpdate", &mutation, variables)
	return err
}

func (i Issue) Link() string {
	return i.URL
}

func (i Issue) Identifier() string {
	return i.ID
}
