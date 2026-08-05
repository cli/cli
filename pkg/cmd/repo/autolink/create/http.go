package create

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/githubrest"
	"github.com/cli/cli/v2/internal/safeurl"
	"github.com/cli/cli/v2/pkg/cmd/repo/autolink/shared"
)

type AutolinkCreator struct {
	GitHubREST func(host string, opts ...githubrest.ClientOption) (*githubrest.Client, error)
}

type AutolinkCreateRequest struct {
	IsAlphanumeric bool   `json:"is_alphanumeric"`
	KeyPrefix      string `json:"key_prefix"`
	URLTemplate    string `json:"url_template"`
}

func (a *AutolinkCreator) Create(ctx context.Context, repo ghrepo.Interface, request AutolinkCreateRequest) (*shared.Autolink, error) {
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "autolinks")
	if err != nil {
		return nil, err
	}

	requestByte, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	requestBody := bytes.NewReader(requestByte)

	client, err := a.GitHubREST(repo.RepoHost())
	if err != nil {
		return nil, err
	}

	var autolink shared.Autolink
	req, err := client.NewRequest(ctx, http.MethodPost, path.String(), requestBody)
	if err != nil {
		return nil, err
	}
	if _, err := client.Do(req, &autolink); err != nil {
		var httpErr *githubrest.ErrorResponse
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			httpErr.Message = "Must have admin rights to Repository."
			return nil, httpErr
		}
		return nil, err
	}

	return &autolink, nil
}
