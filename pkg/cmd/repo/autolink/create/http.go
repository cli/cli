package create

import (
	"bytes"
	"encoding/json"
	"errors"
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
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "autolinks")
	if err != nil {
		return nil, err
	}

	requestByte, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	requestBody := bytes.NewReader(requestByte)

	var autolink shared.Autolink
	// TODO(api-client-rollout)
	// This line of code is part of a mechanical roll out of the api client.
	// As a follow up, consider whether the api client can be injected to this call site, rather than constructed
	err = api.NewClientFromHTTP(a.HTTPClient).REST(repo.RepoHost(), http.MethodPost, path.String(), requestBody, &autolink)
	if err != nil {
		var httpErr api.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			httpErr.Message = "Must have admin rights to Repository."
			return nil, httpErr
		}
		return nil, err
	}

	return &autolink, nil
}
