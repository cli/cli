package set

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/githubrest"
	"github.com/cli/cli/v2/internal/safeurl"
	"github.com/cli/cli/v2/pkg/cmd/secret/shared"
)

type SecretPayload struct {
	EncryptedValue string  `json:"encrypted_value"`
	Visibility     string  `json:"visibility,omitempty"`
	Repositories   []int64 `json:"selected_repository_ids,omitempty"`
	KeyID          string  `json:"key_id"`
}

type DependabotSecretPayload struct {
	EncryptedValue string   `json:"encrypted_value"`
	Visibility     string   `json:"visibility,omitempty"`
	Repositories   []string `json:"selected_repository_ids,omitempty"`
	KeyID          string   `json:"key_id"`
}

type PubKey struct {
	ID  string `json:"key_id"`
	Key string
}

func getPubKey(ctx context.Context, client *githubrest.Client, path safeurl.SafeURL) (*PubKey, error) {
	req, err := client.NewRequest(ctx, http.MethodGet, path.String(), nil)
	if err != nil {
		return nil, err
	}
	pk := PubKey{}
	if _, err := client.Do(req, &pk); err != nil {
		return nil, err
	}
	return &pk, nil
}

func getOrgPublicKey(ctx context.Context, client *githubrest.Client, orgName string, app shared.App) (*PubKey, error) {
	u, err := safeurl.JoinPath("orgs", orgName, string(app), "secrets", "public-key")
	if err != nil {
		return nil, err
	}
	return getPubKey(ctx, client, u)
}

func getUserPublicKey(ctx context.Context, client *githubrest.Client) (*PubKey, error) {
	u, err := safeurl.JoinPath("user", "codespaces", "secrets", "public-key")
	if err != nil {
		return nil, err
	}
	return getPubKey(ctx, client, u)
}

func getRepoPubKey(ctx context.Context, client *githubrest.Client, repo ghrepo.Interface, app shared.App) (*PubKey, error) {
	u, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), string(app), "secrets", "public-key")
	if err != nil {
		return nil, err
	}
	return getPubKey(ctx, client, u)
}

func getEnvPubKey(ctx context.Context, client *githubrest.Client, repo ghrepo.Interface, envName string) (*PubKey, error) {
	u, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "environments", envName, "secrets", "public-key")
	if err != nil {
		return nil, err
	}
	return getPubKey(ctx, client, u)
}

func putSecret(ctx context.Context, client *githubrest.Client, path safeurl.SafeURL, payload interface{}) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to serialize: %w", err)
	}
	req, err := client.NewRequest(ctx, http.MethodPut, path.String(), bytes.NewReader(payloadBytes))
	if err != nil {
		return err
	}
	_, err = client.Do(req, nil)
	return err
}

func putOrgSecret(ctx context.Context, client *githubrest.Client, pk *PubKey, orgName, visibility, secretName, eValue string, repositoryIDs []int64, app shared.App) error {
	path, err := safeurl.JoinPath("orgs", orgName, string(app), "secrets", secretName)
	if err != nil {
		return err
	}

	if app == shared.Dependabot {
		repos := make([]string, len(repositoryIDs))
		for i, id := range repositoryIDs {
			repos[i] = strconv.FormatInt(id, 10)
		}

		payload := DependabotSecretPayload{
			EncryptedValue: eValue,
			KeyID:          pk.ID,
			Repositories:   repos,
			Visibility:     visibility,
		}

		return putSecret(ctx, client, path, payload)
	}

	payload := SecretPayload{
		EncryptedValue: eValue,
		KeyID:          pk.ID,
		Repositories:   repositoryIDs,
		Visibility:     visibility,
	}

	return putSecret(ctx, client, path, payload)
}

func putUserSecret(ctx context.Context, client *githubrest.Client, pk *PubKey, key, eValue string, repositoryIDs []int64) error {
	payload := SecretPayload{
		EncryptedValue: eValue,
		KeyID:          pk.ID,
		Repositories:   repositoryIDs,
	}
	path, err := safeurl.JoinPath("user", "codespaces", "secrets", key)
	if err != nil {
		return err
	}
	return putSecret(ctx, client, path, payload)
}

func putEnvSecret(ctx context.Context, client *githubrest.Client, pk *PubKey, repo ghrepo.Interface, envName string, secretName, eValue string) error {
	payload := SecretPayload{
		EncryptedValue: eValue,
		KeyID:          pk.ID,
	}
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "environments", envName, "secrets", secretName)
	if err != nil {
		return err
	}
	return putSecret(ctx, client, path, payload)
}

func putRepoSecret(ctx context.Context, client *githubrest.Client, pk *PubKey, repo ghrepo.Interface, secretName, eValue string, app shared.App) error {
	payload := SecretPayload{
		EncryptedValue: eValue,
		KeyID:          pk.ID,
	}
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), string(app), "secrets", secretName)
	if err != nil {
		return err
	}
	return putSecret(ctx, client, path, payload)
}
