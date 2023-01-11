package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
)

const (
	projectScope = "project"
	scopesHeader = "X-Oauth-Scopes"
)

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

// If auth token has project scope that means that the host supports
// projectsV2 and also that the auth token has correct permissions to
// manipute them.
func HasProjectsV2Scope(client *http.Client, host string) bool {
	apiEndpoint := ghinstance.RESTPrefix(host)
	res, err := client.Head(apiEndpoint)
	if err != nil {
		return false
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return false
	}
	scopes := res.Header.Get(scopesHeader)
	return strings.Contains(scopes, projectScope)
}
