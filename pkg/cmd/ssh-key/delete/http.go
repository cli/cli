package delete

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/safeurl"
)

type sshKey struct {
	Title string
}

func deleteSSHKey(httpClient *http.Client, host string, keyID string) error {
	url, err := safeurl.JoinPathWithHostPrefix(ghinstance.RESTPrefix(host), "user", "keys", keyID)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("DELETE", url.String(), nil)
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

	return nil
}

func getSSHKey(httpClient *http.Client, host string, keyID string) (*sshKey, error) {
	url, err := safeurl.JoinPathWithHostPrefix(ghinstance.RESTPrefix(host), "user", "keys", keyID)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode > 299 {
		return nil, api.HandleHTTPError(resp)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var key sshKey
	err = json.Unmarshal(b, &key)
	if err != nil {
		return nil, err
	}

	return &key, nil
}
