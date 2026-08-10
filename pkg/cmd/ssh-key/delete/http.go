package delete

import (
	"errors"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/safeurl"
	"github.com/cli/cli/v2/pkg/cmd/ssh-key/shared"
)

type sshKey struct {
	Title string
}

// keyPath returns the REST API path for an SSH key of the given type. GitHub
// stores authentication keys and signing keys at separate endpoints
// (/user/keys and /user/ssh_signing_keys) with independent ID namespaces, so the key type
// determines which endpoint addresses the key.
func keyPath(keyID, keyType string) (safeurl.SafeURL, error) {
	if keyType == shared.SigningKey {
		return safeurl.JoinPath("user", "ssh_signing_keys", keyID)
	}
	return safeurl.JoinPath("user", "keys", keyID)
}

// getSSHKey resolves an SSH key by ID across both the authentication-keys and
// signing-keys endpoints. It tries the authentication-keys endpoint first and
// falls back to the signing-keys endpoint only when the first returns 404, so
// that other errors (e.g. 403 or 500) are surfaced without a misleading second
// request. The returned key type reports which endpoint the key was found at
// and must be passed to deleteSSHKey so the key is removed from the same
// endpoint it was resolved at.
func getSSHKey(httpClient *http.Client, host string, keyID string) (*sshKey, string, error) {
	key, err := fetchSSHKey(httpClient, host, keyID, shared.AuthenticationKey)
	if err == nil {
		return key, shared.AuthenticationKey, nil
	}
	if !isNotFound(err) {
		return nil, "", err
	}
	key, err = fetchSSHKey(httpClient, host, keyID, shared.SigningKey)
	if err != nil {
		return nil, "", err
	}
	return key, shared.SigningKey, nil
}

func fetchSSHKey(httpClient *http.Client, host string, keyID, keyType string) (*sshKey, error) {
	var key sshKey
	path, err := keyPath(keyID, keyType)
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

func deleteSSHKey(httpClient *http.Client, host string, keyID, keyType string) error {
	path, err := keyPath(keyID, keyType)
	if err != nil {
		return err
	}
	// TODO(api-client-rollout)
	// This line of code is part of a mechanical roll out of the api client.
	// As a follow up, consider whether the api client can be injected to this call site, rather than constructed
	return api.NewClientFromHTTP(httpClient).REST(host, http.MethodDelete, path.String(), nil, nil)
}

func isNotFound(err error) bool {
	var httpErr api.HTTPError
	return errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound
}
