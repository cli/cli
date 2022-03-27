package search

import (
	"reflect"
	"strings"
	"time"
)

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
	Assignees         []User           `json:"assignees"`
	Author            User             `json:"user"`
	AuthorAssociation string           `json:"author_association"`
	Body              string           `json:"body"`
	ClosedAt          time.Time        `json:"closed_at"`
	CommentsCount     int              `json:"comments"`
	CreatedAt         time.Time        `json:"created_at"`
	ID                string           `json:"node_id"`
	Labels            []Label          `json:"labels"`
	IsLocked          bool             `json:"locked"`
	Number            int              `json:"number"`
	PullRequestLinks  PullRequestLinks `json:"pull_request"`
	RepositoryURL     string           `json:"repository_url"`
	State             string           `json:"state"`
	Title             string           `json:"title"`
	URL               string           `json:"html_url"`
	UpdatedAt         time.Time        `json:"updated_at"`
}

type PullRequestLinks struct {
	URL string `json:"html_url"`
}

type Label struct {
	Color       string `json:"color"`
	Description string `json:"description"`
	ID          string `json:"node_id"`
	Name        string `json:"name"`
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
			data[f] = map[string]interface{}{
				"id":    repo.Owner.ID,
				"login": repo.Owner.Login,
				"type":  repo.Owner.Type,
				"url":   repo.Owner.URL,
			}
		default:
			sf := fieldByName(v, f)
			data[f] = sf.Interface()
		}
	}
	return data
}

func (issue Issue) IsPullRequest() bool {
	return issue.PullRequestLinks.URL != ""
}

func (issue Issue) ExportData(fields []string) map[string]interface{} {
	v := reflect.ValueOf(issue)
	data := map[string]interface{}{}
	for _, f := range fields {
		switch f {
		case "assignees":
			assignees := make([]interface{}, 0, len(issue.Assignees))
			for _, assignee := range issue.Assignees {
				assignees = append(assignees, map[string]interface{}{
					"id":    assignee.ID,
					"login": assignee.Login,
					"type":  assignee.Type,
				})
			}
			data[f] = assignees
		case "author":
			data[f] = map[string]interface{}{
				"id":    issue.Author.ID,
				"login": issue.Author.Login,
				"type":  issue.Author.Type,
			}
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
			nameWithOwner := strings.Join(comp[len(comp)-2:], "/")
			data[f] = map[string]interface{}{
				"nameWithOwner": nameWithOwner,
			}
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
