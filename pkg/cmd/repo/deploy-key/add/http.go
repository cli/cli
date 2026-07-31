package add

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
)

func uploadDeployKey(httpClient *http.Client, repo ghrepo.Interface, keyFile io.Reader, title string, isWritable bool) error {
	url, err := safeurl.JoinPathWithHostPrefix(ghinstance.RESTPrefix(repo.RepoHost()), "repos", repo.RepoOwner(), repo.RepoName(), "keys")
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

	req, err := http.NewRequest("POST", url.String(), bytes.NewBuffer(payloadBytes))
	if err != nil {
		return err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode > 299 {
		return api.HandleHTTPError(resp)
	}

	_, err = io.Copy(io.Discard, resp.Body)
	if err != nil {
		return err
	}

	return nil
}
