package api

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/cli/cli/v2/internal/ghrepo"
)

type IssuesPayload struct {
	Assigned  IssuesAndTotalCount
	Mentioned IssuesAndTotalCount
	Authored  IssuesAndTotalCount
}

type IssuesAndTotalCount struct {
	Issues       []Issue
	TotalCount   int
	SearchCapped bool
}

type Issue struct {
	Typename         string `json:"__typename"`
	ID               string
	Number           int
	Title            string
	URL              string
	State            string
	StateReason      string
	Closed           bool
	Body             string
	ActiveLockReason string
	Locked           bool
	CreatedAt        time.Time
	UpdatedAt        time.Time
	ClosedAt         *time.Time
	Comments         Comments
	Author           Author
	Assignees        Assignees
	Labels           Labels
	ProjectCards     ProjectCards
	ProjectItems     ProjectItems
	Milestone        *Milestone
	ReactionGroups   ReactionGroups
	IsPinned         bool
}

// return values for Issue.Typename
const (
	TypeIssue       string = "Issue"
	TypePullRequest string = "PullRequest"
)

func (i Issue) IsPullRequest() bool {
	return i.Typename == TypePullRequest
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
	Nodes      []*ProjectInfo
	TotalCount int
}

type ProjectItems struct {
	Nodes []*ProjectV2Item
}

type ProjectInfo struct {
	Project struct {
		Name string `json:"name"`
	} `json:"project"`
	Column struct {
		Name string `json:"name"`
	} `json:"column"`
}

type ProjectV2Item struct {
	ID      string `json:"id"`
	Project struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
}

func (p ProjectCards) ProjectNames() []string {
	names := make([]string, len(p.Nodes))
	for i, c := range p.Nodes {
		names[i] = c.Project.Name
	}
	return names
}

func (p ProjectItems) ProjectTitles() []string {
	titles := make([]string, len(p.Nodes))
	for i, c := range p.Nodes {
		titles[i] = c.Project.Title
	}
	return titles
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
	ID    string
	Name  string
	Login string
}

func (author Author) MarshalJSON() ([]byte, error) {
	if author.ID == "" {
		return json.Marshal(map[string]interface{}{
			"is_bot": true,
			"login":  "app/" + author.Login,
		})
	}
	return json.Marshal(map[string]interface{}{
		"is_bot": false,
		"login":  author.Login,
		"id":     author.ID,
		"name":   author.Name,
	})
}

type CommentAuthor struct {
	Login string `json:"login"`
	// Unfortunately, there is no easy way to add "id" and "name" fields to this struct because it's being
	// used in both shurcool-graphql type queries and string-based queries where the response gets parsed
	// by an ordinary JSON decoder that doesn't understand "graphql" directives via struct tags.
	//	User  *struct {
	//		ID   string
	//		Name string
	//	} `graphql:"... on User"`
}

// IssueCreate creates an issue in a GitHub repository
func IssueCreate(client *Client, repo *Repository, params map[string]interface{}) (*Issue, error) {
	query := `
	mutation IssueCreate($input: CreateIssueInput!) {
		createIssue(input: $input) {
			issue {
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
		case "assigneeIds", "body", "issueTemplate", "labelIds", "milestoneId", "projectIds", "repositoryId", "title":
			inputParams[key] = val
		case "projectV2Ids":
		default:
			return nil, fmt.Errorf("invalid IssueCreate mutation parameter %s", key)
		}
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
	issue := &result.CreateIssue.Issue

	// projectV2 parameters aren't supported in the `createIssue` mutation,
	// so add them after the issue has been created.
	projectV2Ids, ok := params["projectV2Ids"].([]string)
	if ok {
		projectItems := make(map[string]string, len(projectV2Ids))
		for _, p := range projectV2Ids {
			projectItems[p] = issue.ID
		}
		err = UpdateProjectV2Items(client, repo, projectItems, nil)
		if err != nil {
			return issue, err
		}
	}

	return issue, nil
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

	fragments := fmt.Sprintf("fragment issue on Issue{%s}", IssueGraphQL(options.Fields))
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

func (i Issue) Link() string {
	return i.URL
}

func (i Issue) Identifier() string {
	return i.ID
}

func (i Issue) CurrentUserComments() []Comment {
	return i.Comments.CurrentUserComments()
}
