package delete

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/safeurl"
	"github.com/cli/cli/v2/pkg/cmd/ssh-key/shared"
)

type sshKey struct {
	Title string
	Type  string
}

func deleteSSHKey(httpClient *http.Client, host, keyID, keyType string) error {
	path, err := keyAPIPath(keyType, keyID)
	if err != nil {
		return err
	}
	// TODO(api-client-rollout)
	// This line of code is part of a mechanical roll out of the api client.
	// As a follow up, consider whether the api client can be injected to this call site, rather than constructed
	return api.NewClientFromHTTP(httpClient).REST(host, http.MethodDelete, path.String(), nil, nil)
}

func getSSHKey(httpClient *http.Client, host, keyID, keyType string) (*sshKey, error) {
	var key sshKey
	path, err := keyAPIPath(keyType, keyID)
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
	key.Type = keyType
	return &key, nil
}

// resolveSSHKey finds an SSH key by ID.
//
// Authentication and signing keys use independent ID namespaces. When keyType is
// empty, both endpoints are checked. If the ID exists in only one namespace that
// key is returned. If it exists in both, ambiguousKeyError is returned so the
// caller can ask the user to disambiguate (or require --type).
func resolveSSHKey(httpClient *http.Client, host, keyID, keyType string) (*sshKey, error) {
	if keyType != "" {
		return getSSHKey(httpClient, host, keyID, keyType)
	}

	authKey, authErr := getSSHKey(httpClient, host, keyID, shared.AuthenticationKey)
	signingKey, signingErr := getSSHKey(httpClient, host, keyID, shared.SigningKey)

	authFound := authErr == nil
	signingFound := signingErr == nil

	switch {
	case authFound && signingFound:
		return nil, ambiguousKeyError{KeyID: keyID, AuthTitle: authKey.Title, SigningTitle: signingKey.Title}
	case authFound:
		return authKey, nil
	case signingFound:
		return signingKey, nil
	}

	if isNotFound(authErr) && isNotFound(signingErr) {
		return nil, fmt.Errorf("SSH key not found: %s", keyID)
	}
	if !isNotFound(authErr) {
		return nil, authErr
	}
	return nil, signingErr
}

func keyAPIPath(keyType, keyID string) (*safeurl.MutableSafeURL, error) {
	switch keyType {
	case shared.SigningKey:
		return safeurl.JoinPath("user", "ssh_signing_keys", keyID)
	case shared.AuthenticationKey, "":
		return safeurl.JoinPath("user", "keys", keyID)
	default:
		return nil, fmt.Errorf("invalid SSH key type: %q", keyType)
	}
}

func isNotFound(err error) bool {
	var httpErr api.HTTPError
	return errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound
}

type ambiguousKeyError struct {
	KeyID        string
	AuthTitle    string
	SigningTitle string
}

func (e ambiguousKeyError) Error() string {
	return fmt.Sprintf(
		"SSH key ID %s matches both an authentication key (%q) and a signing key (%q); re-run with --type authentication or --type signing",
		e.KeyID, e.AuthTitle, e.SigningTitle,
	)
}
