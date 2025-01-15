package sync

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"

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

type upstreamMergeErr struct{ error }

var missingWorkflowScopeRE = regexp.MustCompile("refusing to allow.*without `workflow` scope")
var missingWorkflowScopeErr = errors.New("Upstream commits contain workflow changes, which require the `workflow` scope to merge. To request it, run: gh auth refresh -s workflow")

func triggerUpstreamMerge(client *api.Client, repo ghrepo.Interface, branch string) (string, error) {
	var payload bytes.Buffer
	if err := json.NewEncoder(&payload).Encode(map[string]interface{}{
		"branch": branch,
	}); err != nil {
		return "", err
	}

	var response struct {
		Message    string `json:"message"`
		MergeType  string `json:"merge_type"`
		BaseBranch string `json:"base_branch"`
	}
	path := fmt.Sprintf("repos/%s/%s/merge-upstream", repo.RepoOwner(), repo.RepoName())
	var httpErr api.HTTPError
	if err := client.REST(repo.RepoHost(), "POST", path, &payload, &response); err != nil {
		if errors.As(err, &httpErr) {
			switch httpErr.StatusCode {
			case http.StatusUnprocessableEntity, http.StatusConflict:
				if missingWorkflowScopeRE.MatchString(httpErr.Message) {
					return "", missingWorkflowScopeErr
				}
				return "", upstreamMergeErr{errors.New(httpErr.Message)}
			}
		}
		return "", err
	}
	return response.BaseBranch, nil
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
