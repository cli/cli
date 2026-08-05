package create

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/cli/cli/v2/internal/githubrest"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
	"github.com/cli/cli/v2/pkg/cmd/repo/autolink/shared"
)

type AutolinkCreator struct {
	HTTPClient *http.Client
}

type AutolinkCreateRequest struct {
	IsAlphanumeric bool   `json:"is_alphanumeric"`
	KeyPrefix      string `json:"key_prefix"`
	URLTemplate    string `json:"url_template"`
}

func (a *AutolinkCreator) Create(repo ghrepo.Interface, request AutolinkCreateRequest) (*shared.Autolink, error) {
	client, err := api.NewRESTClient(a.HTTPClient, repo.RepoHost())
	if err != nil {
		return nil, err
	}

	url, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "autolinks")
	if err != nil {
		return nil, err
	}

	requestByte, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}

	req, err := client.NewRequest(context.Background(), http.MethodPost, url.String(), bytes.NewReader(requestByte))
	if err != nil {
		return nil, err
	}

	var autolink shared.Autolink
	if _, err := client.Do(req, &autolink); err != nil {
		return nil, handleAutolinkCreateError(err)
	}

	return &autolink, nil
}

// handleAutolinkCreateError rewrites the 404 the API returns for a caller
// without admin rights, which is otherwise indistinguishable from a missing
// repository.
func handleAutolinkCreateError(err error) error {
	var httpErr *githubrest.ErrorResponse
	if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
		httpErr.Message = "Must have admin rights to Repository."
		return httpErr
	}
	return err
}
