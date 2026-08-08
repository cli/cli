package delete

import (
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/safeurl"
)

type sshKey struct {
	Title string
}

func deleteSSHKey(httpClient *http.Client, host string, keyID string) error {
	path, err := safeurl.JoinPath("user", "keys", keyID)
	if err != nil {
		return err
	}
	// TODO(api-client-rollout)
	// This line of code is part of a mechanical roll out of the api client.
	// As a follow up, consider whether the api client can be injected to this call site, rather than constructed
	return api.NewClientFromHTTP(httpClient).REST(host, http.MethodDelete, path.String(), nil, nil)
}

func getSSHKey(httpClient *http.Client, host string, keyID string) (*sshKey, error) {
	var key sshKey
	path, err := safeurl.JoinPath("user", "keys", keyID)
	if err != nil {
		return nil, err
	}
	// TODO(api-client-rollout)
	// This line of code is part of a mechanical roll out of the api client.
	// As a follow up, consider whether the api client can be injected to this call site, rather than constructed
	err = api.NewClientFromHTTP(httpClient).REST(host, http.MethodGet, path.String(), nil, &key)
	if err != nil {
		return nil, err
	}

	return &key, nil
}
