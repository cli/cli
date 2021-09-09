package sync

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
)

type commit struct {
	Ref    string `json:"ref"`
	NodeID string `json:"node_id"`
	URL    string `json:"url"`
	Object struct {
		Type string `json:"type"`
		SHA  string `json:"sha"`
		URL  string `json:"url"`
	} `json:"object"`
}

func latestCommit(client *api.Client, repo ghrepo.Interface, branch string) (commit, error) {
	var response commit
	path := fmt.Sprintf("repos/%s/%s/git/refs/heads/%s", repo.RepoOwner(), repo.RepoName(), branch)
	err := client.REST(repo.RepoHost(), "GET", path, nil, &response)
	return response, err
}

func syncFork(client *api.Client, repo ghrepo.Interface, branch, SHA string, force bool) error {
	path := fmt.Sprintf("repos/%s/%s/git/refs/heads/%s", repo.RepoOwner(), repo.RepoName(), branch)
	body := map[string]interface{}{
		"sha":   SHA,
		"force": force,
	}
	requestByte, err := json.Marshal(body)
	if err != nil {
		return err
	}
	requestBody := bytes.NewReader(requestByte)
	return client.REST(repo.RepoHost(), "PATCH", path, requestBody, nil)
}
