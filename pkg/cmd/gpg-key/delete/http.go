package delete

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/safeurl"
)

type gpgKey struct {
	ID    int64
	KeyID string `json:"key_id"`
}

func deleteGPGKey(httpClient *http.Client, host, id string) error {
	url, err := safeurl.JoinPathWithHostPrefix(ghinstance.RESTPrefix(host), "user", "gpg_keys", id)
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

func getGPGKeys(httpClient *http.Client, host string) ([]gpgKey, error) {
	u, err := safeurl.JoinPathWithHostPrefix(ghinstance.RESTPrefix(host), "user", "gpg_keys")
	if err != nil {
		return nil, err
	}
	u.SetQuery("per_page", "100")
	req, err := http.NewRequest("GET", u.String(), nil)
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

	var keys []gpgKey
	err = json.Unmarshal(b, &keys)
	if err != nil {
		return nil, err
	}

	return keys, nil
}
