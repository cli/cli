package delete

import (
	"context"
	"net/http"

	"github.com/cli/cli/v2/internal/githubrest"
	"github.com/cli/cli/v2/internal/safeurl"
)

type sshKey struct {
	Title string
}

func deleteSSHKey(ctx context.Context, client *githubrest.Client, keyID string) error {
	path, err := safeurl.JoinPath("user", "keys", keyID)
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

func getSSHKey(ctx context.Context, client *githubrest.Client, keyID string) (*sshKey, error) {
	var key sshKey
	path, err := safeurl.JoinPath("user", "keys", keyID)
	if err != nil {
		return nil, err
	}
	req, err := client.NewRequest(ctx, http.MethodGet, path.String(), nil)
	if err != nil {
		return nil, err
	}
	if _, err := client.Do(req, &key); err != nil {
		return nil, err
	}

	return &key, nil
}
