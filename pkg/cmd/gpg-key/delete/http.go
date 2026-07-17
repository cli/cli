package delete

import (
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/api"
)

type gpgKey struct {
	ID    int64
	KeyID string `json:"key_id"`
}

func deleteGPGKey(httpClient *http.Client, host, id string) error {
	path := fmt.Sprintf("user/gpg_keys/%s", id)
	return api.NewClientFromHTTP(httpClient).REST(host, "DELETE", path, nil, nil)
}

func getGPGKeys(httpClient *http.Client, host string) ([]gpgKey, error) {
	var keys []gpgKey
	if err := api.NewClientFromHTTP(httpClient).REST(host, "GET", "user/gpg_keys?per_page=100", nil, &keys); err != nil {
		return nil, err
	}
	return keys, nil
}
