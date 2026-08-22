package delete

import (
	"errors"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/safeurl"
)

type sshKey struct {
	Title string
	Type  string // "authentication" or "signing"
}

func deleteSSHKey(httpClient *http.Client, host string, keyID string) error {
	// Try the authentication-keys endpoint first; if the key is a signing key,
	// GitHub returns 404 from /user/keys/{id} and we fall through to the
	// signing-keys endpoint. See https://github.com/cli/cli/issues/14064.
	authPath, err := safeurl.JoinPath("user", "keys", keyID)
	if err != nil {
		return err
	}
	// TODO(api-client-rollout)
	// This line of code is part of a mechanical roll out of the api client.
	// As a follow up, consider whether the api client can be injected to this call site, rather than constructed
	err = api.NewClientFromHTTP(httpClient).REST(host, http.MethodDelete, authPath.String(), nil, nil)
	if err == nil {
		return nil
	}
	var httpErr api.HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusNotFound {
		return err
	}

	signingPath, err := safeurl.JoinPath("user", "ssh_signing_keys", keyID)
	if err != nil {
		return err
	}
	return api.NewClientFromHTTP(httpClient).REST(host, http.MethodDelete, signingPath.String(), nil, nil)
}

func getSSHKey(httpClient *http.Client, host string, keyID string) (*sshKey, error) {
	var key sshKey
	authPath, err := safeurl.JoinPath("user", "keys", keyID)
	if err != nil {
		return nil, err
	}
	// TODO(api-client-rollout)
	// This line of code is part of a mechanical roll out of the api client.
	// As a follow up, consider whether the api client can be injected to this call site, rather than constructed
	err = api.NewClientFromHTTP(httpClient).REST(host, http.MethodGet, authPath.String(), nil, &key)
	if err == nil {
		key.Type = "authentication"
		return &key, nil
	}
	var httpErr api.HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusNotFound {
		return nil, err
	}

	// Fall through to the signing-keys endpoint.
	key = sshKey{}
	signingPath, err := safeurl.JoinPath("user", "ssh_signing_keys", keyID)
	if err != nil {
		return nil, err
	}
	err = api.NewClientFromHTTP(httpClient).REST(host, http.MethodGet, signingPath.String(), nil, &key)
	if err != nil {
		return nil, err
	}
	key.Type = "signing"
	return &key, nil
}
