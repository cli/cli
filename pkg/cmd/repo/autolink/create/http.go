package create

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
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
	url := ghinstance.RESTPrefix(repo.RepoHost()) + path

	requestByte, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	requestBody := bytes.NewReader(requestByte)

	req, err := http.NewRequest(http.MethodPost, url, requestBody)
	if err != nil {
		return nil, err
	}

	resp, err := a.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	err = handleAutolinkCreateError(resp)

	if err != nil {
		return nil, err
	}

	var autolink shared.Autolink

	err = json.NewDecoder(resp.Body).Decode(&autolink)
	if err != nil {
		return nil, err
	}

	return &autolink, nil
}

func handleAutolinkCreateError(resp *http.Response) error {
	switch resp.StatusCode {
	case http.StatusCreated:
		return nil
	case http.StatusNotFound:
		err := api.HandleHTTPError(resp)
		var httpErr api.HTTPError
		if errors.As(err, &httpErr) {
			httpErr.Message = "Must have admin rights to Repository."
			return httpErr
		}
		return err
	default:
		return api.HandleHTTPError(resp)
	}
}
