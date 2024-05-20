package api

import (
	"fmt"
	"strings"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/shurcooL/githubv4"
)

const (
	errorProjectsV2ReadScope         = "field requires one of the following scopes: ['read:project']"
	errorProjectsV2UserField         = "Field 'projectsV2' doesn't exist on type 'User'"
	errorProjectsV2RepositoryField   = "Field 'projectsV2' doesn't exist on type 'Repository'"
	errorProjectsV2OrganizationField = "Field 'projectsV2' doesn't exist on type 'Organization'"
	errorProjectsV2IssueField        = "Field 'projectItems' doesn't exist on type 'Issue'"
	errorProjectsV2PullRequestField  = "Field 'projectItems' doesn't exist on type 'PullRequest'"
)

type ProjectV2 struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Number       int    `json:"number"`
	ResourcePath string `json:"resourcePath"`
	Closed       bool   `json:"closed"`
	URL          string `json:"url"`
}

// UpdateProjectV2Items uses the addProjectV2ItemById and the deleteProjectV2Item mutations
// to add and delete items from projects. The addProjectItems and deleteProjectItems arguments are
// mappings between a project and an item. This function can be used across multiple projects
// and items. Note that the deleteProjectV2Item mutation requires the item id from the project not
// the global id.
func UpdateProjectV2Items(client *Client, repo ghrepo.Interface, addProjectItems, deleteProjectItems map[string]string) error {
	l := len(addProjectItems) + len(deleteProjectItems)
	if l == 0 {
		return nil
	}
	inputs := make([]string, 0, l)
	mutations := make([]string, 0, l)
	variables := make(map[string]interface{}, l)
	var i int

	for project, item := range addProjectItems {
		inputs = append(inputs, fmt.Sprintf("$input_%03d: AddProjectV2ItemByIdInput!", i))
		mutations = append(mutations, fmt.Sprintf("add_%03d: addProjectV2ItemById(input: $input_%03d) { item { id } }", i, i))
		variables[fmt.Sprintf("input_%03d", i)] = map[string]interface{}{"contentId": item, "projectId": project}
		i++
	}

	for project, item := range deleteProjectItems {
		inputs = append(inputs, fmt.Sprintf("$input_%03d: DeleteProjectV2ItemInput!", i))
		mutations = append(mutations, fmt.Sprintf("delete_%03d: deleteProjectV2Item(input: $input_%03d) { deletedItemId }", i, i))
		variables[fmt.Sprintf("input_%03d", i)] = map[string]interface{}{"itemId": item, "projectId": project}
		i++
	}

	query := fmt.Sprintf(`mutation UpdateProjectV2Items(%s) {%s}`, strings.Join(inputs, " "), strings.Join(mutations, " "))

	return client.GraphQL(repo.RepoHost(), query, variables, nil)
}

// ProjectsV2ItemsForIssue fetches all ProjectItems for an issue.
func ProjectsV2ItemsForIssue(client *Client, repo ghrepo.Interface, issue *Issue) error {
	type projectV2ItemStatus struct {
		StatusFragment struct {
			OptionID string `json:"optionId"`
			Name     string `json:"name"`
		} `graphql:"... on ProjectV2ItemFieldSingleSelectValue"`
	}

	type projectV2Item struct {
		ID      string `json:"id"`
		Project struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		}
		Status projectV2ItemStatus `graphql:"status:fieldValueByName(name: \"Status\")"`
	}

	type response struct {
		Repository struct {
			Issue struct {
				ProjectItems struct {
					Nodes    []*projectV2Item
					PageInfo struct {
						HasNextPage bool
						EndCursor   string
					}
				} `graphql:"projectItems(first: 100, after: $endCursor)"`
			} `graphql:"issue(number: $number)"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}
	variables := map[string]interface{}{
		"owner":     githubv4.String(repo.RepoOwner()),
		"name":      githubv4.String(repo.RepoName()),
		"number":    githubv4.Int(issue.Number),
		"endCursor": (*githubv4.String)(nil),
	}
	var items ProjectItems
	for {
		var query response
		err := client.Query(repo.RepoHost(), "IssueProjectItems", &query, variables)
		if err != nil {
			return err
		}
		for _, projectItemNode := range query.Repository.Issue.ProjectItems.Nodes {
			items.Nodes = append(items.Nodes, &ProjectV2Item{
				ID: projectItemNode.ID,
				Project: ProjectV2ItemProject{
					ID:    projectItemNode.Project.ID,
					Title: projectItemNode.Project.Title,
				},
				Status: ProjectV2ItemStatus{
					OptionID: projectItemNode.Status.StatusFragment.OptionID,
					Name:     projectItemNode.Status.StatusFragment.Name,
				},
			})
		}

		if !query.Repository.Issue.ProjectItems.PageInfo.HasNextPage {
			break
		}
		variables["endCursor"] = githubv4.String(query.Repository.Issue.ProjectItems.PageInfo.EndCursor)
	}
	issue.ProjectItems = items
	return nil
}

// ProjectsV2ItemsForPullRequest fetches all ProjectItems for a pull request.
func ProjectsV2ItemsForPullRequest(client *Client, repo ghrepo.Interface, pr *PullRequest) error {
	type projectV2ItemStatus struct {
		StatusFragment struct {
			OptionID string `json:"optionId"`
			Name     string `json:"name"`
		} `graphql:"... on ProjectV2ItemFieldSingleSelectValue"`
	}

	type projectV2Item struct {
		ID      string `json:"id"`
		Project struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		}
		Status projectV2ItemStatus `graphql:"status:fieldValueByName(name: \"Status\")"`
	}

	type response struct {
		Repository struct {
			PullRequest struct {
				ProjectItems struct {
					Nodes    []*projectV2Item
					PageInfo struct {
						HasNextPage bool
						EndCursor   string
					}
				} `graphql:"projectItems(first: 100, after: $endCursor)"`
			} `graphql:"pullRequest(number: $number)"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}
	variables := map[string]interface{}{
		"owner":     githubv4.String(repo.RepoOwner()),
		"name":      githubv4.String(repo.RepoName()),
		"number":    githubv4.Int(pr.Number),
		"endCursor": (*githubv4.String)(nil),
	}
	var items ProjectItems
	for {
		var query response
		err := client.Query(repo.RepoHost(), "PullRequestProjectItems", &query, variables)
		if err != nil {
			return err
		}

		for _, projectItemNode := range query.Repository.PullRequest.ProjectItems.Nodes {
			items.Nodes = append(items.Nodes, &ProjectV2Item{
				ID: projectItemNode.ID,
				Project: ProjectV2ItemProject{
					ID:    projectItemNode.Project.ID,
					Title: projectItemNode.Project.Title,
				},
				Status: ProjectV2ItemStatus{
					OptionID: projectItemNode.Status.StatusFragment.OptionID,
					Name:     projectItemNode.Status.StatusFragment.Name,
				},
			})
		}

		if !query.Repository.PullRequest.ProjectItems.PageInfo.HasNextPage {
			break
		}
		variables["endCursor"] = githubv4.String(query.Repository.PullRequest.ProjectItems.PageInfo.EndCursor)
	}
	pr.ProjectItems = items
	return nil
}

// OrganizationProjectsV2 fetches all open projectsV2 for an organization.
func OrganizationProjectsV2(client *Client, repo ghrepo.Interface) ([]ProjectV2, error) {
	type responseData struct {
		Organization struct {
			ProjectsV2 struct {
				Nodes    []ProjectV2
				PageInfo struct {
					HasNextPage bool
					EndCursor   string
				}
			} `graphql:"projectsV2(first: 100, orderBy: {field: TITLE, direction: ASC}, after: $endCursor, query: $query)"`
		} `graphql:"organization(login: $owner)"`
	}

	variables := map[string]interface{}{
		"owner":     githubv4.String(repo.RepoOwner()),
		"endCursor": (*githubv4.String)(nil),
		"query":     githubv4.String("is:open"),
	}

	var projectsV2 []ProjectV2
	for {
		var query responseData
		err := client.Query(repo.RepoHost(), "OrganizationProjectV2List", &query, variables)
		if err != nil {
			return nil, err
		}

		projectsV2 = append(projectsV2, query.Organization.ProjectsV2.Nodes...)

		if !query.Organization.ProjectsV2.PageInfo.HasNextPage {
			break
		}
		variables["endCursor"] = githubv4.String(query.Organization.ProjectsV2.PageInfo.EndCursor)
	}

	return projectsV2, nil
}

// RepoProjectsV2 fetches all open projectsV2 for a repository.
func RepoProjectsV2(client *Client, repo ghrepo.Interface) ([]ProjectV2, error) {
	type responseData struct {
		Repository struct {
			ProjectsV2 struct {
				Nodes    []ProjectV2
				PageInfo struct {
					HasNextPage bool
					EndCursor   string
				}
			} `graphql:"projectsV2(first: 100, orderBy: {field: TITLE, direction: ASC}, after: $endCursor, query: $query)"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}

	variables := map[string]interface{}{
		"owner":     githubv4.String(repo.RepoOwner()),
		"name":      githubv4.String(repo.RepoName()),
		"endCursor": (*githubv4.String)(nil),
		"query":     githubv4.String("is:open"),
	}

	var projectsV2 []ProjectV2
	for {
		var query responseData
		err := client.Query(repo.RepoHost(), "RepositoryProjectV2List", &query, variables)
		if err != nil {
			return nil, err
		}

		projectsV2 = append(projectsV2, query.Repository.ProjectsV2.Nodes...)

		if !query.Repository.ProjectsV2.PageInfo.HasNextPage {
			break
		}
		variables["endCursor"] = githubv4.String(query.Repository.ProjectsV2.PageInfo.EndCursor)
	}

	return projectsV2, nil
}

// CurrentUserProjectsV2 fetches all open projectsV2 for current user.
func CurrentUserProjectsV2(client *Client, hostname string) ([]ProjectV2, error) {
	type responseData struct {
		Viewer struct {
			ProjectsV2 struct {
				Nodes    []ProjectV2
				PageInfo struct {
					HasNextPage bool
					EndCursor   string
				}
			} `graphql:"projectsV2(first: 100, orderBy: {field: TITLE, direction: ASC}, after: $endCursor, query: $query)"`
		} `graphql:"viewer"`
	}

	variables := map[string]interface{}{
		"endCursor": (*githubv4.String)(nil),
		"query":     githubv4.String("is:open"),
	}

	var projectsV2 []ProjectV2
	for {
		var query responseData
		err := client.Query(hostname, "UserProjectV2List", &query, variables)
		if err != nil {
			return nil, err
		}

		projectsV2 = append(projectsV2, query.Viewer.ProjectsV2.Nodes...)

		if !query.Viewer.ProjectsV2.PageInfo.HasNextPage {
			break
		}
		variables["endCursor"] = githubv4.String(query.Viewer.ProjectsV2.PageInfo.EndCursor)
	}

	return projectsV2, nil
}

// When querying ProjectsV2 fields we generally dont want to show the user
// scope errors and field does not exist errors. ProjectsV2IgnorableError
// checks against known error strings to see if an error can be safely ignored.
// Due to the fact that the GraphQLClient can return multiple types of errors
// this uses brittle string comparison to check against the known error strings.
func ProjectsV2IgnorableError(err error) bool {
	msg := err.Error()
	if strings.Contains(msg, errorProjectsV2ReadScope) ||
		strings.Contains(msg, errorProjectsV2UserField) ||
		strings.Contains(msg, errorProjectsV2RepositoryField) ||
		strings.Contains(msg, errorProjectsV2OrganizationField) ||
		strings.Contains(msg, errorProjectsV2IssueField) ||
		strings.Contains(msg, errorProjectsV2PullRequestField) {
		return true
	}
	return false
}
