package create

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
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
	path := fmt.Sprintf("repos/%s/%s/autolinks", repo.RepoOwner(), repo.RepoName())

	requestByte, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}

	var autolink shared.Autolink
	err = api.NewClientFromHTTP(a.HTTPClient).REST(repo.RepoHost(), http.MethodPost, path, bytes.NewReader(requestByte), &autolink)
	if err != nil {
		return nil, handleAutolinkCreateError(err)
	}

	return &autolink, nil
}

func handleAutolinkCreateError(err error) error {
	var httpErr api.HTTPError
	if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
		httpErr.Message = "Must have admin rights to Repository."
		return httpErr
	}
	return err
}
