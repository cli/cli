package add

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
)

func uploadDeployKey(httpClient *http.Client, repo ghrepo.Interface, keyFile io.Reader, title string, isWritable bool) error {
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "keys")
	if err != nil {
		return err
	}

	keyBytes, err := io.ReadAll(keyFile)
	if err != nil {
		return err
	}

	payload := map[string]any{
		"title":     title,
		"key":       string(keyBytes),
		"read_only": !isWritable,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	// TODO(api-client-rollout)
	// This line of code is part of a mechanical roll out of the api client.
	// As a follow up, consider whether the api client can be injected to this call site, rather than constructed
	return api.NewClientFromHTTP(httpClient).REST(repo.RepoHost(), "POST", path.String(), bytes.NewBuffer(payloadBytes), nil)
}
