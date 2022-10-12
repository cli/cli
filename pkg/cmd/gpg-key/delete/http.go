package delete

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
)

type gpgKey struct {
	ID    int
	KeyID string `json:"key_id"`
}

func deleteGPGKey(httpClient *http.Client, host, id string) error {
	url := fmt.Sprintf("%suser/gpg_keys/%s", ghinstance.RESTPrefix(host), id)
	req, err := http.NewRequest("DELETE", url, nil)
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
	resource := "user/gpg_keys"
	url := fmt.Sprintf("%s%s?per_page=%d", ghinstance.RESTPrefix(host), resource, 100)
	req, err := http.NewRequest("GET", url, nil)
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
