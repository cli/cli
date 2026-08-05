package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/githubrest"
	"github.com/cli/cli/v2/internal/safeurl"
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

func latestCommit(ctx context.Context, client *githubrest.Client, repo ghrepo.Interface, branch string) (commit, error) {
	var response commit
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "git", "refs", fmt.Sprintf("heads/%s", branch))
	if err != nil {
		return response, err
	}
	req, err := client.NewRequest(ctx, http.MethodGet, path.String(), nil)
	if err != nil {
		return response, err
	}
	_, err = client.Do(req, &response)
	return response, err
}

type upstreamMergeErr struct{ error }

var missingWorkflowScopeRE = regexp.MustCompile("refusing to allow.*without `workflow(s)?` (scope|permission)")
var missingWorkflowScopeErr = errors.New("Upstream commits contain workflow changes, which require the `workflow` scope or permission to merge. To request it, run: gh auth refresh -s workflow")

func triggerUpstreamMerge(ctx context.Context, client *githubrest.Client, repo ghrepo.Interface, branch string) (string, error) {
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
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "merge-upstream")
	if err != nil {
		return "", err
	}
	req, err := client.NewRequest(ctx, http.MethodPost, path.String(), &payload)
	if err != nil {
		return "", err
	}
	var httpErr *githubrest.ErrorResponse
	if _, err := client.Do(req, &response); err != nil {
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

func syncFork(ctx context.Context, client *githubrest.Client, repo ghrepo.Interface, branch, SHA string, force bool) error {
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "git", "refs", fmt.Sprintf("heads/%s", branch))
	if err != nil {
		return err
	}
	body := map[string]interface{}{
		"sha":   SHA,
		"force": force,
	}
	requestByte, err := json.Marshal(body)
	if err != nil {
		return err
	}
	requestBody := bytes.NewReader(requestByte)
	req, err := client.NewRequest(ctx, http.MethodPatch, path.String(), requestBody)
	if err != nil {
		return err
	}
	_, err = client.Do(req, nil)
	return err
}
