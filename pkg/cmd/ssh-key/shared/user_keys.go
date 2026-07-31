package shared

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
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
	u, err := safeurl.JoinPathWithHostPrefix(ghinstance.RESTPrefix(host), "user", "keys")
	if err != nil {
		return nil, err
	}
	if userHandle != "" {
		u, err = safeurl.JoinPathWithHostPrefix(ghinstance.RESTPrefix(host), "users", userHandle, "keys")
		if err != nil {
			return nil, err
		}
	}
	u.SetQuery("per_page", "100")

	keys, err := getUserKeys(httpClient, u)

	if err != nil {
		return nil, err
	}

	for i := 0; i < len(keys); i++ {
		keys[i].Type = AuthenticationKey
	}

	return keys, nil
}

func UserSigningKeys(httpClient *http.Client, host, userHandle string) ([]sshKey, error) {
	u, err := safeurl.JoinPathWithHostPrefix(ghinstance.RESTPrefix(host), "user", "ssh_signing_keys")
	if err != nil {
		return nil, err
	}
	if userHandle != "" {
		u, err = safeurl.JoinPathWithHostPrefix(ghinstance.RESTPrefix(host), "users", userHandle, "ssh_signing_keys")
		if err != nil {
			return nil, err
		}
	}
	u.SetQuery("per_page", "100")

	keys, err := getUserKeys(httpClient, u)

	if err != nil {
		return nil, err
	}

	for i := 0; i < len(keys); i++ {
		keys[i].Type = SigningKey
	}

	return keys, nil
}

func getUserKeys(httpClient *http.Client, u safeurl.SafeURL) ([]sshKey, error) {
	req, err := http.NewRequest("GET", u.String(), nil)
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

	var keys []sshKey
	err = json.Unmarshal(b, &keys)
	if err != nil {
		return nil, err
	}

	return keys, nil
}
