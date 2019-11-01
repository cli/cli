package api

import "fmt"

func IssueCreate(client *Client, ghRepo Repo, params map[string]interface{}) (*Issue, error) {
	repoId, err := GitHubRepoId(client, ghRepo)
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
		"repositoryId": repoId,
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

func GitHubRepoId(client *Client, ghRepo Repo) (string, error) {
	owner := ghRepo.RepoOwner()
	repo := ghRepo.RepoName()

	query := `
		query FindRepoID($owner:String!, $name:String!) {
			repository(owner:$owner, name:$name) {
				id
			}
	}`
	variables := map[string]interface{}{
		"owner": owner,
		"name":  repo,
	}

	result := struct {
		Repository struct {
			Id string
		}
	}{}
	err := client.GraphQL(query, variables, &result)
	if err != nil || result.Repository.Id == "" {
		return "", fmt.Errorf("failed to determine GH repo ID: %s", err)
	}

	return result.Repository.Id, nil
}
