package shared

import (
	"net/http"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/safeurl"
)

const (
	AuthenticationKey = "authentication"
	SigningKey        = "signing"
)

type sshKey struct {
	ID        int64
	Key       string
	Title     string
	Type      string
	CreatedAt time.Time `json:"created_at"`
}

func UserKeys(httpClient *http.Client, host, userHandle string) ([]sshKey, error) {
	u, err := safeurl.JoinPath("user", "keys")
	if err != nil {
		return nil, err
	}
	if userHandle != "" {
		u, err = safeurl.JoinPath("users", userHandle, "keys")
		if err != nil {
			return nil, err
		}
	}
	u.SetQuery("per_page", "100")

	keys, err := getUserKeys(httpClient, host, u)

	if err != nil {
		return nil, err
	}

	for i := range keys {
		keys[i].Type = AuthenticationKey
	}

	return keys, nil
}

func UserSigningKeys(httpClient *http.Client, host, userHandle string) ([]sshKey, error) {
	u, err := safeurl.JoinPath("user", "ssh_signing_keys")
	if err != nil {
		return nil, err
	}
	if userHandle != "" {
		u, err = safeurl.JoinPath("users", userHandle, "ssh_signing_keys")
		if err != nil {
			return nil, err
		}
	}
	u.SetQuery("per_page", "100")

	keys, err := getUserKeys(httpClient, host, u)

	if err != nil {
		return nil, err
	}

	for i := range keys {
		keys[i].Type = SigningKey
	}

	return keys, nil
}

func getUserKeys(httpClient *http.Client, hostname string, u safeurl.SafeURL) ([]sshKey, error) {
	var keys []sshKey
	// TODO(api-client-rollout)
	// This line of code is part of a mechanical roll out of the api client.
	// As a follow up, consider whether the api client can be injected to this call site, rather than constructed
	err := api.NewClientFromHTTP(httpClient).REST(hostname, http.MethodGet, u.String(), nil, &keys)
	if err != nil {
		return nil, err
	}

	return keys, nil
}
