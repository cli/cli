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
	"visibility",
	"watchersCount",
}

type RepositoriesResult struct {
	IncompleteResults bool         `json:"incomplete_results"`
	Items             []Repository `json:"items"`
	Total             int          `json:"total_count"`
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
	ID              int64     `json:"id"`
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
	UpdatedAt       time.Time `json:"updated_at"`
	Visibility      string    `json:"visibility"`
	WatchersCount   int       `json:"watchers_count"`
}

type License struct {
	HTMLURL string `json:"html_url"`
	Key     string `json:"key"`
	Name    string `json:"name"`
	URL     string `json:"url"`
}

type User struct {
	GravatarID string `json:"gravatar_id"`
	ID         int64  `json:"id"`
	Login      string `json:"login"`
	SiteAdmin  bool   `json:"site_admin"`
	Type       string `json:"type"`
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
