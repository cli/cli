package api

import (
	"fmt"

	"github.com/cli/cli/v2/internal/ghrepo"
)

const issueFieldDefinitionsWithoutMultiSelect = `
	...on IssueFieldText{id,name,dataType}
	...on IssueFieldNumber{id,name,dataType}
	...on IssueFieldDate{id,name,dataType}
	...on IssueFieldSingleSelect{id,name,dataType,options{id,name}}
`

const issueFieldMultiSelectDefinition = `...on IssueFieldMultiSelect{id,name,dataType,options{id,name}}`

// RepoIssueFields fetches all issue fields available to a repository.
func RepoIssueFields(client *Client, repo ghrepo.Interface, includeMultiSelect bool) ([]IssueFieldDefinition, error) {
	definitions := issueFieldDefinitionsWithoutMultiSelect
	if includeMultiSelect {
		definitions += issueFieldMultiSelectDefinition
	}
	query := fmt.Sprintf(`
	query RepositoryIssueFields($owner: String!, $name: String!, $endCursor: String) {
		repository(owner: $owner, name: $name) {
			issueFields(first: 100, after: $endCursor) {
				nodes{%s}
				pageInfo{hasNextPage,endCursor}
			}
		}
	}`, definitions)
	variables := map[string]interface{}{
		"owner": repo.RepoOwner(),
		"name":  repo.RepoName(),
	}

	var fields []IssueFieldDefinition
	for {
		var result struct {
			Repository struct {
				IssueFields issueFieldDefinitionConnection
			}
		}
		if err := client.GraphQL(repo.RepoHost(), query, variables, &result); err != nil {
			return nil, err
		}
		fields = append(fields, result.Repository.IssueFields.Nodes...)
		if !result.Repository.IssueFields.PageInfo.HasNextPage {
			return fields, nil
		}
		variables["endCursor"] = result.Repository.IssueFields.PageInfo.EndCursor
	}
}

type issueFieldDefinitionConnection struct {
	Nodes    []IssueFieldDefinition
	PageInfo struct {
		HasNextPage bool
		EndCursor   string
	}
}
