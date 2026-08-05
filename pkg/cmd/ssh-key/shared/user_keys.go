package shared

import (
	"context"
	"net/http"
	"time"

	"github.com/cli/cli/v2/internal/githubrest"
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

func UserKeys(ctx context.Context, client *githubrest.Client, userHandle string, reqOpts ...githubrest.RequestOption) ([]sshKey, error) {
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

	keys, err := getUserKeys(ctx, client, u, reqOpts...)

	if err != nil {
		return nil, err
	}

	for i := 0; i < len(keys); i++ {
		keys[i].Type = AuthenticationKey
	}

	return keys, nil
}

func UserSigningKeys(ctx context.Context, client *githubrest.Client, userHandle string, reqOpts ...githubrest.RequestOption) ([]sshKey, error) {
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

	keys, err := getUserKeys(ctx, client, u, reqOpts...)

	if err != nil {
		return nil, err
	}

	for i := 0; i < len(keys); i++ {
		keys[i].Type = SigningKey
	}

	return keys, nil
}

func getUserKeys(ctx context.Context, client *githubrest.Client, u safeurl.SafeURL, reqOpts ...githubrest.RequestOption) ([]sshKey, error) {
	var keys []sshKey
	req, err := client.NewRequest(ctx, http.MethodGet, u.String(), nil, reqOpts...)
	if err != nil {
		return nil, err
	}
	if _, err := client.Do(req, &keys); err != nil {
		return nil, err
	}

	return keys, nil
}
