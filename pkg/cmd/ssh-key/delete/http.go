package delete

import (
	"errors"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/safeurl"
)

type sshKey struct {
	Title        string
	IsSigningKey bool
}

func deleteSSHKey(httpClient *http.Client, host string, keyID string, isSigningKey bool) error {
	var (
		path *safeurl.MutableSafeURL
		err  error
	)
	if isSigningKey {
		path, err = safeurl.JoinPath("user", "ssh_signing_keys", keyID)
	} else {
		path, err = safeurl.JoinPath("user", "keys", keyID)
	}
	if err != nil {
		return err
	}
	// TODO(api-client-rollout)
	// This line of code is part of a mechanical roll out of the api client.
	// As a follow up, consider whether the api client can be injected to this call site, rather than constructed
	return api.NewClientFromHTTP(httpClient).REST(host, http.MethodDelete, path.String(), nil, nil)
}

func getSSHKey(httpClient *http.Client, host string, keyID string) (*sshKey, error) {
	key, err := getAuthenticationKey(httpClient, host, keyID)
	if err == nil {
		return key, nil
	}

	var httpErr api.HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusNotFound {
		return nil, err
	}

	// Not found among authentication keys; try signing keys, which live under
	// a separate endpoint with their own ID namespace.
	return getSigningKey(httpClient, host, keyID)
}

func getAuthenticationKey(httpClient *http.Client, host string, keyID string) (*sshKey, error) {
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

func getSigningKey(httpClient *http.Client, host string, keyID string) (*sshKey, error) {
	var key sshKey
	path, err := safeurl.JoinPath("user", "ssh_signing_keys", keyID)
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
	key.IsSigningKey = true

	return &key, nil
}
