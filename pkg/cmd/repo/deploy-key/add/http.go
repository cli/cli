package add

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
)

var scopesError = errors.New("insufficient OAuth scopes")

func uploadDeployKey(httpClient *http.Client, repo ghrepo.Interface, keyFile io.ReadCloser, title string, isWritable bool) error {
	path := fmt.Sprintf("repos/%s/%s/keys", repo.RepoOwner(), repo.RepoName())
	url := ghinstance.RESTPrefix(repo.RepoHost()) + path

	defer keyFile.Close()
	keyBytes, err := ioutil.ReadAll(keyFile)
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

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return scopesError
	} else if resp.StatusCode > 299 {
		return api.HandleHTTPError(resp)
	}

	_, err = io.Copy(ioutil.Discard, resp.Body)
	if err != nil {
		return err
	}

	return nil
}
