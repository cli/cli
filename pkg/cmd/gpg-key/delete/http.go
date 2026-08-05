package delete

import (
	"context"
	"net/http"

	"github.com/cli/cli/v2/internal/githubrest"
	"github.com/cli/cli/v2/internal/safeurl"
)

type gpgKey struct {
	ID    int64
	KeyID string `json:"key_id"`
}

func deleteGPGKey(ctx context.Context, client *githubrest.Client, id string) error {
	path, err := safeurl.JoinPath("user", "gpg_keys", id)
	if err != nil {
		return err
	}
	req, err := client.NewRequest(ctx, http.MethodDelete, path.String(), nil)
	if err != nil {
		return err
	}
	_, err = client.Do(req, nil)
	return err
}

func getGPGKeys(ctx context.Context, client *githubrest.Client) ([]gpgKey, error) {
	u, err := safeurl.JoinPath("user", "gpg_keys")
	if err != nil {
		return nil, err
	}
	u.SetQuery("per_page", "100")

	var keys []gpgKey
	req, err := client.NewRequest(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	if _, err := client.Do(req, &keys); err != nil {
		return nil, err
	}
	return keys, nil
}
