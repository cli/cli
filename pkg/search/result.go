package search

import (
	"reflect"
	"strings"
	"time"
)

var CommitFields = []string{
	"author",
	"commit",
	"committer",
	"sha",
	"id",
	"parents",
	"repository",
	"url",
}

var RepositoryFields = []string{
	"createdAt",
	"defaultBranch",
	"description",
	"forksCount",
	"fullName",
	"hasDownloads",
	"hasIssues",
	"hasPages",
	"hasProjects",
	"hasWiki",
	"homepage",
	"id",
	"isArchived",
	"isDisabled",
	"isFork",
	"isPrivate",
	"language",
	"license",
	"name",
	"openIssuesCount",
	"owner",
	"pushedAt",
	"size",
	"stargazersCount",
	"updatedAt",
	"url",
	"visibility",
	"watchersCount",
}

var IssueFields = []string{
	"assignees",
	"author",
	"authorAssociation",
	"body",
	"closedAt",
	"commentsCount",
	"createdAt",
	"id",
	"isLocked",
	"isPullRequest",
	"labels",
	"number",
	"repository",
	"state",
	"title",
	"updatedAt",
	"url",
}

var PullRequestFields = append(IssueFields,
	"isDraft",
)

type CommitsResult struct {
	IncompleteResults bool     `json:"incomplete_results"`
	Items             []Commit `json:"items"`
	Total             int      `json:"total_count"`
}

type RepositoriesResult struct {
	IncompleteResults bool         `json:"incomplete_results"`
	Items             []Repository `json:"items"`
	Total             int          `json:"total_count"`
}

type IssuesResult struct {
	IncompleteResults bool    `json:"incomplete_results"`
	Items             []Issue `json:"items"`
	Total             int     `json:"total_count"`
}

type Commit struct {
	Author    User       `json:"author"`
	Committer User       `json:"committer"`
	ID        string     `json:"node_id"`
	Info      CommitInfo `json:"commit"`
	Parents   []Parent   `json:"parents"`
	Repo      Repository `json:"repository"`
	Sha       string     `json:"sha"`
	URL       string     `json:"html_url"`
}

type CommitInfo struct {
	Author       CommitUser `json:"author"`
	CommentCount int        `json:"comment_count"`
	Committer    CommitUser `json:"committer"`
	Message      string     `json:"message"`
	Tree         Tree       `json:"tree"`
}

type CommitUser struct {
	Date  time.Time `json:"date"`
	Email string    `json:"email"`
	Name  string    `json:"name"`
}

type Tree struct {
	Sha string `json:"sha"`
}

type Parent struct {
	Sha string `json:"sha"`
	URL string `json:"html_url"`
}

type Repository struct {
	CreatedAt       time.Time `json:"created_at"`
	DefaultBranch   string    `json:"default_branch"`
	Description     string    `json:"description"`
	ForksCount      int       `json:"forks_count"`
	FullName        string    `json:"full_name"`
	HasDownloads    bool      `json:"has_downloads"`
	HasIssues       bool      `json:"has_issues"`
	HasPages        bool      `json:"has_pages"`
	HasProjects     bool      `json:"has_projects"`
	HasWiki         bool      `json:"has_wiki"`
	Homepage        string    `json:"homepage"`
	ID              string    `json:"node_id"`
	IsArchived      bool      `json:"archived"`
	IsDisabled      bool      `json:"disabled"`
	IsFork          bool      `json:"fork"`
	IsPrivate       bool      `json:"private"`
	Language        string    `json:"language"`
	License         License   `json:"license"`
	MasterBranch    string    `json:"master_branch"`
	Name            string    `json:"name"`
	OpenIssuesCount int       `json:"open_issues_count"`
	Owner           User      `json:"owner"`
	PushedAt        time.Time `json:"pushed_at"`
	Size            int       `json:"size"`
	StargazersCount int       `json:"stargazers_count"`
	URL             string    `json:"html_url"`
	UpdatedAt       time.Time `json:"updated_at"`
	Visibility      string    `json:"visibility"`
	WatchersCount   int       `json:"watchers_count"`
}

type License struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

type User struct {
	GravatarID string `json:"gravatar_id"`
	ID         string `json:"node_id"`
	Login      string `json:"login"`
	SiteAdmin  bool   `json:"site_admin"`
	Type       string `json:"type"`
	URL        string `json:"html_url"`
}

type Issue struct {
	Assignees         []User    `json:"assignees"`
	Author            User      `json:"user"`
	AuthorAssociation string    `json:"author_association"`
	Body              string    `json:"body"`
	ClosedAt          time.Time `json:"closed_at"`
	CommentsCount     int       `json:"comments"`
	CreatedAt         time.Time `json:"created_at"`
	ID                string    `json:"node_id"`
	Labels            []Label   `json:"labels"`
	// This is a PullRequest field which does not appear in issue results,
	// but lives outside the PullRequest object.
	IsDraft       *bool       `json:"draft,omitempty"`
	IsLocked      bool        `json:"locked"`
	Number        int         `json:"number"`
	PullRequest   PullRequest `json:"pull_request"`
	RepositoryURL string      `json:"repository_url"`
	// StateInternal should not be used directly. Use State() instead.
	StateInternal string    `json:"state"`
	StateReason   string    `json:"state_reason"`
	Title         string    `json:"title"`
	URL           string    `json:"html_url"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type PullRequest struct {
	URL      string    `json:"html_url"`
	MergedAt time.Time `json:"merged_at"`
}

type Label struct {
	Color       string `json:"color"`
	Description string `json:"description"`
	ID          string `json:"node_id"`
	Name        string `json:"name"`
}

func (u User) IsBot() bool {
	// copied from api/queries_issue.go
	// would ideally be shared, but it would require coordinating a "user"
	// abstraction in a bunch of places.
	return u.ID == ""
}

func (u User) ExportData() map[string]interface{} {
	isBot := u.IsBot()
	login := u.Login
	if isBot {
		login = "app/" + login
	}
	return map[string]interface{}{
		"id":     u.ID,
		"login":  login,
		"type":   u.Type,
		"url":    u.URL,
		"is_bot": isBot,
	}
}

func (commit Commit) ExportData(fields []string) map[string]interface{} {
	v := reflect.ValueOf(commit)
	data := map[string]interface{}{}
	for _, f := range fields {
		switch f {
		case "author":
			data[f] = commit.Author.ExportData()
		case "commit":
			info := commit.Info
			data[f] = map[string]interface{}{
				"author": map[string]interface{}{
					"date":  info.Author.Date,
					"email": info.Author.Email,
					"name":  info.Author.Name,
				},
				"committer": map[string]interface{}{
					"date":  info.Committer.Date,
					"email": info.Committer.Email,
					"name":  info.Committer.Name,
				},
				"comment_count": info.CommentCount,
				"message":       info.Message,
				"tree":          map[string]interface{}{"sha": info.Tree.Sha},
			}
		case "committer":
			data[f] = commit.Committer.ExportData()
		case "parents":
			parents := make([]interface{}, 0, len(commit.Parents))
			for _, parent := range commit.Parents {
				parents = append(parents, map[string]interface{}{
					"sha": parent.Sha,
					"url": parent.URL,
				})
			}
			data[f] = parents
		case "repository":
			repo := commit.Repo
			data[f] = map[string]interface{}{
				"description": repo.Description,
				"fullName":    repo.FullName,
				"name":        repo.Name,
				"id":          repo.ID,
				"isFork":      repo.IsFork,
				"isPrivate":   repo.IsPrivate,
				"owner":       repo.Owner.ExportData(),
				"url":         repo.URL,
			}
		default:
			sf := fieldByName(v, f)
			data[f] = sf.Interface()
		}
	}
	return data
}

func (repo Repository) ExportData(fields []string) map[string]interface{} {
	v := reflect.ValueOf(repo)
	data := map[string]interface{}{}
	for _, f := range fields {
		switch f {
		case "license":
			data[f] = map[string]interface{}{
				"key":  repo.License.Key,
				"name": repo.License.Name,
				"url":  repo.License.URL,
			}
		case "owner":
			data[f] = repo.Owner.ExportData()
		default:
			sf := fieldByName(v, f)
			data[f] = sf.Interface()
		}
	}
	return data
}

// The state of an issue or a pull request, may be either open or closed.
// For a pull request, the "merged" state is inferred from a value for merged_at and
// which we take return instead of the "closed" state.
func (issue Issue) State() string {
	if !issue.PullRequest.MergedAt.IsZero() {
		return "merged"
	}
	return issue.StateInternal
}

func (issue Issue) IsPullRequest() bool {
	return issue.PullRequest.URL != ""
}

func (issue Issue) ExportData(fields []string) map[string]interface{} {
	v := reflect.ValueOf(issue)
	data := map[string]interface{}{}
	for _, f := range fields {
		switch f {
		case "assignees":
			assignees := make([]interface{}, 0, len(issue.Assignees))
			for _, assignee := range issue.Assignees {
				assignees = append(assignees, assignee.ExportData())
			}
			data[f] = assignees
		case "author":
			data[f] = issue.Author.ExportData()
		case "isPullRequest":
			data[f] = issue.IsPullRequest()
		case "labels":
			labels := make([]interface{}, 0, len(issue.Labels))
			for _, label := range issue.Labels {
				labels = append(labels, map[string]interface{}{
					"color":       label.Color,
					"description": label.Description,
					"id":          label.ID,
					"name":        label.Name,
				})
			}
			data[f] = labels
		case "repository":
			comp := strings.Split(issue.RepositoryURL, "/")
			name := comp[len(comp)-1]
			nameWithOwner := strings.Join(comp[len(comp)-2:], "/")
			data[f] = map[string]interface{}{
				"name":          name,
				"nameWithOwner": nameWithOwner,
			}
		case "state":
			data[f] = issue.State()
		default:
			sf := fieldByName(v, f)
			data[f] = sf.Interface()
		}
	}
	return data
}

func fieldByName(v reflect.Value, field string) reflect.Value {
	return v.FieldByNameFunc(func(s string) bool {
		return strings.EqualFold(field, s)
	})
}
