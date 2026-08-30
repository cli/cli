package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/shurcooL/githubv4"
)

// RenameBranch renames a branch in the repository via the REST API.
// It returns the new branch name.
func RenameBranch(client *Client, repo ghrepo.Interface, oldName, newName string) (string, error) {
	input := map[string]string{"new_name": newName}
	body := &bytes.Buffer{}
	enc := json.NewEncoder(body)
	if err := enc.Encode(input); err != nil {
		return "", err
	}

	path := fmt.Sprintf("repos/%s/%s/branches/%s/rename",
		repo.RepoOwner(), repo.RepoName(), url.PathEscape(oldName))

	var result struct {
		Name string `json:"name"`
	}
	err := client.REST(repo.RepoHost(), "POST", path, body, &result)
	if err != nil {
		return "", err
	}
	if result.Name == "" {
		return "", fmt.Errorf("rename returned an empty branch name")
	}
	return result.Name, nil
}

// UpdatePullRequestBase changes the base branch of a pull request via the
// GraphQL updatePullRequest mutation.
func UpdatePullRequestBase(client *Client, repo ghrepo.Interface, prID, baseRefName string) error {
	var mutation struct {
		UpdatePullRequest struct {
			ClientMutationID string
		} `graphql:"updatePullRequest(input: $input)"`
	}

	variables := map[string]any{
		"input": githubv4.UpdatePullRequestInput{
			PullRequestID: githubv4.ID(prID),
			BaseRefName:   githubv4.NewString(githubv4.String(baseRefName)),
		},
	}

	return client.Mutate(repo.RepoHost(), "UpdatePullRequestBase", &mutation, variables)
}
