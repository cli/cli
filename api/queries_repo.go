package api

import (
	"fmt"

	"github.com/pkg/errors"
)

// Repository contains information about a GitHub repo
type Repository struct {
	ID               string
	HasIssuesEnabled bool
}

// GitHubRepo looks up the node ID of a named repository
func GitHubRepo(client *Client, ghRepo Repo) (*Repository, error) {
	owner := ghRepo.RepoOwner()
	repo := ghRepo.RepoName()

	query := `
	query($owner: String!, $name: String!) {
		repository(owner: $owner, name: $name) {
			id
			hasIssuesEnabled
		}
	}`
	variables := map[string]interface{}{
		"owner": owner,
		"name":  repo,
	}

	result := struct {
		Repository Repository
	}{}
	err := client.GraphQL(query, variables, &result)

	if err != nil || result.Repository.ID == "" {
		newErr := fmt.Errorf("failed to determine repository ID for '%s/%s'", owner, repo)
		if err != nil {
			newErr = errors.Wrap(err, newErr.Error())
		}
		return nil, newErr
	}

	return &result.Repository, nil
}
