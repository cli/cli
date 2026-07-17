package add

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
)

func uploadDeployKey(httpClient *http.Client, repo ghrepo.Interface, keyFile io.Reader, title string, isWritable bool) error {
	keyBytes, err := io.ReadAll(keyFile)
	if err != nil {
		return err
	}

	payload := map[string]interface{}{
		"title":     title,
		"key":       string(keyBytes),
		"read_only": !isWritable,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("repos/%s/%s/keys", repo.RepoOwner(), repo.RepoName())
	return api.NewClientFromHTTP(httpClient).REST(repo.RepoHost(), "POST", path, bytes.NewReader(payloadBytes), nil)
}
