package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/shurcooL/githubv4"
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
	AssignedActors   AssignedActors
	Labels           Labels
	ProjectCards     ProjectCards
	ProjectItems     ProjectItems
	Milestone        *Milestone
	ReactionGroups   ReactionGroups
	IsPinned         bool

	IssueType        *IssueType
	Parent           *LinkedIssue
	SubIssues        SubIssues
	SubIssuesSummary SubIssuesSummary
	BlockedBy        LinkedIssueConnection
	Blocking         LinkedIssueConnection

	ClosedByPullRequestsReferences ClosedByPullRequestsReferences
}

// IssueType represents an issue type configured for a repository.
type IssueType struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Color       string `json:"color"`
}

// LinkedIssue represents a related issue (parent, sub-issue, or relationship target).
type LinkedIssue struct {
	ID         string `json:"id"`
	Number     int    `json:"number"`
	Title      string `json:"title"`
	URL        string `json:"url"`
	State      string `json:"state"`
	Repository struct {
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"repository"`
}

// SubIssues is a connection of sub-issues with a total count.
type SubIssues struct {
	Nodes      []LinkedIssue `json:"nodes"`
	TotalCount int           `json:"totalCount"`
}

// SubIssuesSummary contains completion stats for sub-issues.
type SubIssuesSummary struct {
	Total            int     `json:"total"`
	Completed        int     `json:"completed"`
	PercentCompleted float64 `json:"percentCompleted"`
}

// LinkedIssueConnection is a connection of related issues (blocked-by or blocking).
type LinkedIssueConnection struct {
	Nodes      []LinkedIssue `json:"nodes"`
	TotalCount int           `json:"totalCount"`
}

type ClosedByPullRequestsReferences struct {
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

type AssignedActors struct {
	Nodes      []Actor
	TotalCount int
}

func (a AssignedActors) Logins() []string {
	logins := make([]string, len(a.Nodes))
	for i, a := range a.Nodes {
		logins[i] = a.Login
	}
	return logins
}

// DisplayNames returns a list of display names for the assigned actors.
func (a AssignedActors) DisplayNames() []string {
	// These display names are used for populating the "default" assigned actors
	// from the AssignedActors type. But, this is only one piece of the puzzle
	// as later, other queries will fetch the full list of possible assignable
	// actors from the repository, and the two lists will be reconciled.
	//
	// It's important that the display names are the same between the defaults
	// (the values returned here) and the full list (the values returned by
	// other repository queries). Any discrepancy would result in an
	// "invalid default", which means an assigned actor will not be matched
	// to an assignable actor and not presented as a "default" selection.
	// Not being presented as a default would cause the actor to be potentially
	// unassigned if the edits were submitted.
	//
	// To prevent this, we need shared logic to look up an actor's display name.
	// However, our API types between assignedActors and the full list of
	// assignableActors are different. So, as an attempt to maintain
	// consistency we convert the assignedActors to the same types as the
	// repository's assignableActors, treating the assignableActors DisplayName
	// methods as the sources of truth.
	// TODO KW: make this comment less of a wall of text if needed.
	var displayNames []string
	for _, a := range a.Nodes {
		if a.TypeName == "User" {
			u := NewAssignableUser(
				a.ID,
				a.Login,
				a.Name,
			)
			displayNames = append(displayNames, u.DisplayName())
		} else if a.TypeName == "Bot" {
			b := NewAssignableBot(
				a.ID,
				a.Login,
			)
			displayNames = append(displayNames, b.DisplayName())
		}
	}
	return displayNames
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
	Nodes      []*ProjectV2Item
	TotalCount int
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
	Project ProjectV2ItemProject
	Status  ProjectV2ItemStatus
}

type ProjectV2ItemProject struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type ProjectV2ItemStatus struct {
	OptionID string `json:"optionId"`
	Name     string `json:"name"`
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

// DisplayName returns a user-friendly name via actorDisplayName.
func (a Author) DisplayName() string {
	return actorDisplayName("", a.Login, a.Name)
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

// DisplayName returns a user-friendly name via actorDisplayName.
func (a CommentAuthor) DisplayName() string {
	return actorDisplayName("", a.Login, "")
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
		case "projectV2Ids", "assigneeLogins":
			// handled after issue creation
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

	// Assign users using login-based mutation when ApiActorsSupported is true (github.com).
	if assigneeLogins, ok := params["assigneeLogins"].([]string); ok && len(assigneeLogins) > 0 {
		err := ReplaceActorsForAssignableByLogin(client, repo, issue.ID, assigneeLogins)
		if err != nil {
			return issue, err
		}
	}

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

// UpdateIssueIssueType sets or clears the issue type on an issue. Pass an
// empty issueTypeID to clear the issue type.
func UpdateIssueIssueType(client *Client, hostname string, issueID string, issueTypeID string) error {
	type UpdateIssueIssueTypeInput struct {
		IssueID     githubv4.ID  `json:"issueId"`
		IssueTypeID *githubv4.ID `json:"issueTypeId"`
	}

	var mutation struct {
		UpdateIssueIssueType struct {
			Issue struct {
				ID string
			}
		} `graphql:"updateIssueIssueType(input: $input)"`
	}

	var typeID *githubv4.ID
	if issueTypeID != "" {
		id := githubv4.ID(issueTypeID)
		typeID = &id
	}

	variables := map[string]interface{}{
		"input": UpdateIssueIssueTypeInput{
			IssueID:     githubv4.ID(issueID),
			IssueTypeID: typeID,
		},
	}

	return client.Mutate(hostname, "UpdateIssueIssueType", &mutation, variables)
}

// AddSubIssue adds a sub-issue to a parent issue.
func AddSubIssue(client *Client, hostname string, parentID string, subIssueID string, replaceParent bool) error {
	type AddSubIssueInput struct {
		IssueID       githubv4.ID      `json:"issueId"`
		SubIssueID    githubv4.ID      `json:"subIssueId"`
		ReplaceParent githubv4.Boolean `json:"replaceParent"`
	}

	var mutation struct {
		AddSubIssue struct {
			Issue struct {
				ID string
			}
		} `graphql:"addSubIssue(input: $input)"`
	}

	variables := map[string]interface{}{
		"input": AddSubIssueInput{
			IssueID:       githubv4.ID(parentID),
			SubIssueID:    githubv4.ID(subIssueID),
			ReplaceParent: githubv4.Boolean(replaceParent),
		},
	}

	return client.Mutate(hostname, "AddSubIssue", &mutation, variables)
}

// RemoveSubIssue removes a sub-issue from a parent issue.
func RemoveSubIssue(client *Client, hostname string, parentID string, subIssueID string) error {
	type RemoveSubIssueInput struct {
		IssueID    githubv4.ID `json:"issueId"`
		SubIssueID githubv4.ID `json:"subIssueId"`
	}

	var mutation struct {
		RemoveSubIssue struct {
			Issue struct {
				ID string
			}
		} `graphql:"removeSubIssue(input: $input)"`
	}

	variables := map[string]interface{}{
		"input": RemoveSubIssueInput{
			IssueID:    githubv4.ID(parentID),
			SubIssueID: githubv4.ID(subIssueID),
		},
	}

	return client.Mutate(hostname, "RemoveSubIssue", &mutation, variables)
}

// AddBlockedBy marks an issue as blocked by another issue.
func AddBlockedBy(client *Client, hostname string, issueID string, blockingIssueID string) error {
	type AddBlockedByInput struct {
		IssueID         githubv4.ID `json:"issueId"`
		BlockingIssueID githubv4.ID `json:"blockingIssueId"`
	}

	var mutation struct {
		AddBlockedBy struct {
			Issue struct {
				ID string
			}
		} `graphql:"addBlockedBy(input: $input)"`
	}

	variables := map[string]interface{}{
		"input": AddBlockedByInput{
			IssueID:         githubv4.ID(issueID),
			BlockingIssueID: githubv4.ID(blockingIssueID),
		},
	}

	return client.Mutate(hostname, "AddBlockedBy", &mutation, variables)
}

// RemoveBlockedBy removes a "blocked by" relationship between two issues.
func RemoveBlockedBy(client *Client, hostname string, issueID string, blockingIssueID string) error {
	type RemoveBlockedByInput struct {
		IssueID         githubv4.ID `json:"issueId"`
		BlockingIssueID githubv4.ID `json:"blockingIssueId"`
	}

	var mutation struct {
		RemoveBlockedBy struct {
			Issue struct {
				ID string
			}
		} `graphql:"removeBlockedBy(input: $input)"`
	}

	variables := map[string]interface{}{
		"input": RemoveBlockedByInput{
			IssueID:         githubv4.ID(issueID),
			BlockingIssueID: githubv4.ID(blockingIssueID),
		},
	}

	return client.Mutate(hostname, "RemoveBlockedBy", &mutation, variables)
}

// DeferredUpdateIssueOptions updates an issue with mutations unsupported by the
// standard issue update mutations. All ID fields are node IDs.
type DeferredUpdateIssueOptions struct {
	IssueID  string
	Hostname string

	IssueTypeID     string
	RemoveIssueType bool

	ParentID              string
	ReplaceExistingParent bool
	RemoveParentID        string

	AddSubIssueIDs    []string
	RemoveSubIssueIDs []string

	AddBlockedByIDs    []string
	RemoveBlockedByIDs []string

	// AddBlockingIDs / RemoveBlockingIDs name issues that this issue
	// blocks. They are applied via the addBlockedBy / removeBlockedBy
	// mutations with the arguments swapped.
	AddBlockingIDs    []string
	RemoveBlockingIDs []string
}

// DeferredUpdateIssue runs issue mutations described by opts in
// parallel and returns any failures as a single joined error so a single
// failure does not abort the rest.
func DeferredUpdateIssue(client *Client, opts DeferredUpdateIssueOptions) error {
	var mutations []func() error

	if opts.IssueTypeID != "" || opts.RemoveIssueType {
		mutations = append(mutations, func() error {
			return UpdateIssueIssueType(client, opts.Hostname, opts.IssueID, opts.IssueTypeID)
		})
	}

	if opts.ParentID != "" {
		mutations = append(mutations, func() error {
			return AddSubIssue(client, opts.Hostname, opts.ParentID, opts.IssueID, opts.ReplaceExistingParent)
		})
	} else if opts.RemoveParentID != "" {
		mutations = append(mutations, func() error {
			return RemoveSubIssue(client, opts.Hostname, opts.RemoveParentID, opts.IssueID)
		})
	}

	for _, id := range opts.AddSubIssueIDs {
		mutations = append(mutations, func() error {
			return AddSubIssue(client, opts.Hostname, opts.IssueID, id, true)
		})
	}
	for _, id := range opts.RemoveSubIssueIDs {
		mutations = append(mutations, func() error {
			return RemoveSubIssue(client, opts.Hostname, opts.IssueID, id)
		})
	}

	for _, id := range opts.AddBlockedByIDs {
		mutations = append(mutations, func() error {
			return AddBlockedBy(client, opts.Hostname, opts.IssueID, id)
		})
	}
	for _, id := range opts.RemoveBlockedByIDs {
		mutations = append(mutations, func() error {
			return RemoveBlockedBy(client, opts.Hostname, opts.IssueID, id)
		})
	}

	for _, id := range opts.AddBlockingIDs {
		mutations = append(mutations, func() error {
			// blocking is the inverse of blocked-by: this issue blocks `id`,
			// expressed as `id` is blocked by this issue.
			return AddBlockedBy(client, opts.Hostname, id, opts.IssueID)
		})
	}
	for _, id := range opts.RemoveBlockingIDs {
		mutations = append(mutations, func() error {
			return RemoveBlockedBy(client, opts.Hostname, id, opts.IssueID)
		})
	}

	if len(mutations) == 0 {
		return nil
	}

	errCh := make(chan error, len(mutations))
	var wg sync.WaitGroup
	for _, m := range mutations {
		wg.Add(1)
		go func(m func() error) {
			defer wg.Done()
			if err := m(); err != nil {
				errCh <- err
			}
		}(m)
	}
	wg.Wait()
	close(errCh)

	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// RepoIssueTypes fetches the available issue types for a repository.
func RepoIssueTypes(client *Client, repo ghrepo.Interface) ([]IssueType, error) {
	query := `
	query RepositoryIssueTypes($owner: String!, $name: String!) {
		repository(owner: $owner, name: $name) {
			issueTypes(first: 50) {
				nodes { id, name, description, color }
			}
		}
	}`
	variables := map[string]interface{}{
		"owner": repo.RepoOwner(),
		"name":  repo.RepoName(),
	}
	var result struct {
		Repository struct {
			IssueTypes struct {
				Nodes []IssueType
			}
		}
	}
	err := client.GraphQL(repo.RepoHost(), query, variables, &result)
	if err != nil {
		return nil, err
	}
	return result.Repository.IssueTypes.Nodes, nil
}

// IssueNodeID fetches the node ID for an issue given its number and repository.
func IssueNodeID(client *Client, repo ghrepo.Interface, number int) (string, error) {
	query := `
	query IssueNodeID($owner: String!, $name: String!, $number: Int!) {
		repository(owner: $owner, name: $name) {
			issue(number: $number) {
				id
			}
		}
	}`
	variables := map[string]interface{}{
		"owner":  repo.RepoOwner(),
		"name":   repo.RepoName(),
		"number": number,
	}
	var result struct {
		Repository struct {
			Issue struct {
				ID string
			}
		}
	}
	err := client.GraphQL(repo.RepoHost(), query, variables, &result)
	if err != nil {
		return "", err
	}
	return result.Repository.Issue.ID, nil
}
