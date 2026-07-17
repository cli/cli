package shared

import (
	"fmt"
	"net/http"
	"time"

	"github.com/cli/cli/v2/api"
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
	resource := "user/keys"
	if userHandle != "" {
		resource = fmt.Sprintf("users/%s/keys", userHandle)
	}

	keys, err := getUserKeys(httpClient, host, resource+"?per_page=100")
	if err != nil {
		return nil, err
	}

	for i := 0; i < len(keys); i++ {
		keys[i].Type = AuthenticationKey
	}

	return keys, nil
}

func UserSigningKeys(httpClient *http.Client, host, userHandle string) ([]sshKey, error) {
	resource := "user/ssh_signing_keys"
	if userHandle != "" {
		resource = fmt.Sprintf("users/%s/ssh_signing_keys", userHandle)
	}

	keys, err := getUserKeys(httpClient, host, resource+"?per_page=100")
	if err != nil {
		return nil, err
	}

	for i := 0; i < len(keys); i++ {
		keys[i].Type = SigningKey
	}

	return keys, nil
}

func getUserKeys(httpClient *http.Client, host, path string) ([]sshKey, error) {
	var keys []sshKey
	if err := api.NewClientFromHTTP(httpClient).REST(host, "GET", path, nil, &keys); err != nil {
		return nil, err
	}
	return keys, nil
}
