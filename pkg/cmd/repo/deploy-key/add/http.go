package add

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/githubrest"
	"github.com/cli/cli/v2/internal/safeurl"
)

func uploadDeployKey(ctx context.Context, client *githubrest.Client, repo ghrepo.Interface, keyFile io.Reader, title string, isWritable bool) error {
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "keys")
	if err != nil {
		return err
	}

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

	req, err := client.NewRequest(ctx, http.MethodPost, path.String(), bytes.NewBuffer(payloadBytes))
	if err != nil {
		return err
	}
	_, err = client.Do(req, nil)
	return err
}
