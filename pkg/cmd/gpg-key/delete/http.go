package delete

import (
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/safeurl"
)

type gpgKey struct {
	ID    int64
	KeyID string `json:"key_id"`
}

func deleteGPGKey(httpClient *http.Client, host, id string) error {
	path, err := safeurl.JoinPath("user", "gpg_keys", id)
	if err != nil {
		return err
	}
	// TODO(api-client-rollout)
	// This line of code is part of a mechanical roll out of the api client.
	// As a follow up, consider whether the api client can be injected to this call site, rather than constructed
	return api.NewClientFromHTTP(httpClient).REST(host, "DELETE", path.String(), nil, nil)
}

func getGPGKeys(httpClient *http.Client, host string) ([]gpgKey, error) {
	u, err := safeurl.JoinPath("user", "gpg_keys")
	if err != nil {
		return nil, err
	}
	u.SetQuery("per_page", "100")

	var keys []gpgKey
	// TODO(api-client-rollout)
	// This line of code is part of a mechanical roll out of the api client.
	// As a follow up, consider whether the api client can be injected to this call site, rather than constructed
	err = api.NewClientFromHTTP(httpClient).REST(host, "GET", u.String(), nil, &keys)
	if err != nil {
		return nil, err
	}
	return keys, nil
}
