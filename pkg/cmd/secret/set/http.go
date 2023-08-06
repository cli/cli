package set

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
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

func getPubKey(client *api.Client, host, path string) (*PubKey, error) {
	pk := PubKey{}
	err := client.REST(host, "GET", path, nil, &pk)
	if err != nil {
		return nil, err
	}
	return &pk, nil
}

func getOrgPublicKey(client *api.Client, host, orgName string, app shared.App) (*PubKey, error) {
	return getPubKey(client, host, fmt.Sprintf("orgs/%s/%s/secrets/public-key", orgName, app))
}

func getUserPublicKey(client *api.Client, host string) (*PubKey, error) {
	return getPubKey(client, host, "user/codespaces/secrets/public-key")
}

func getRepoPubKey(client *api.Client, repo ghrepo.Interface, app shared.App) (*PubKey, error) {
	return getPubKey(client, repo.RepoHost(), fmt.Sprintf("repos/%s/%s/secrets/public-key",
		ghrepo.FullName(repo), app))
}

func getEnvPubKey(client *api.Client, repo ghrepo.Interface, envName string) (*PubKey, error) {
	return getPubKey(client, repo.RepoHost(), fmt.Sprintf("repos/%s/environments/%s/secrets/public-key",
		ghrepo.FullName(repo), envName))
}

func putSecret(client *api.Client, host, path string, payload interface{}) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to serialize: %w", err)
	}
	requestBody := bytes.NewReader(payloadBytes)

	return client.REST(host, "PUT", path, requestBody, nil)
}

func putOrgSecret(client *api.Client, host string, pk *PubKey, orgName, visibility, secretName, eValue string, repositoryIDs []int64, app shared.App) error {
	path := fmt.Sprintf("orgs/%s/%s/secrets/%s", orgName, app, secretName)

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

		return putSecret(client, host, path, payload)
	}

	payload := SecretPayload{
		EncryptedValue: eValue,
		KeyID:          pk.ID,
		Repositories:   repositoryIDs,
		Visibility:     visibility,
	}

	return putSecret(client, host, path, payload)
}

func putUserSecret(client *api.Client, host string, pk *PubKey, key, eValue string, repositoryIDs []int64) error {
	payload := SecretPayload{
		EncryptedValue: eValue,
		KeyID:          pk.ID,
		Repositories:   repositoryIDs,
	}
	path := fmt.Sprintf("user/codespaces/secrets/%s", key)
	return putSecret(client, host, path, payload)
}

func putEnvSecret(client *api.Client, pk *PubKey, repo ghrepo.Interface, envName string, secretName, eValue string) error {
	payload := SecretPayload{
		EncryptedValue: eValue,
		KeyID:          pk.ID,
	}
	path := fmt.Sprintf("repos/%s/environments/%s/secrets/%s", ghrepo.FullName(repo), envName, secretName)
	return putSecret(client, repo.RepoHost(), path, payload)
}

func putRepoSecret(client *api.Client, pk *PubKey, repo ghrepo.Interface, secretName, eValue string, app shared.App) error {
	payload := SecretPayload{
		EncryptedValue: eValue,
		KeyID:          pk.ID,
	}
	path := fmt.Sprintf("repos/%s/%s/secrets/%s", ghrepo.FullName(repo), app, secretName)
	return putSecret(client, repo.RepoHost(), path, payload)
}
