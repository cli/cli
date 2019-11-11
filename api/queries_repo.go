package api

import "fmt"

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
