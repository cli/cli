package set

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

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

// The Codespaces Secret API currently expects repositories IDs as strings
type CodespacesSecretPayload struct {
	EncryptedValue string   `json:"encrypted_value"`
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
	payload := SecretPayload{
		EncryptedValue: eValue,
		KeyID:          pk.ID,
		Repositories:   repositoryIDs,
		Visibility:     visibility,
	}
	path := fmt.Sprintf("orgs/%s/%s/secrets/%s", orgName, app, secretName)

	return putSecret(client, host, path, payload)
}

func putUserSecret(client *api.Client, host string, pk *PubKey, key, eValue string, repositoryIDs []int64) error {
	payload := CodespacesSecretPayload{
		EncryptedValue: eValue,
		KeyID:          pk.ID,
	}

	if len(repositoryIDs) > 0 {
		repositoryStringIDs := make([]string, len(repositoryIDs))
		for i, id := range repositoryIDs {
			repositoryStringIDs[i] = strconv.FormatInt(id, 10)
		}
		payload.Repositories = repositoryStringIDs
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

// This does similar logic to `api.RepoNetwork`, but without the overfetching.
func mapRepoToID(client *api.Client, host string, repositories []ghrepo.Interface) ([]int64, error) {
	queries := make([]string, 0, len(repositories))
	for i, repo := range repositories {
		queries = append(queries, fmt.Sprintf(`
			repo_%03d: repository(owner: %q, name: %q) {
				databaseId
			}
		`, i, repo.RepoOwner(), repo.RepoName()))
	}

	query := fmt.Sprintf(`query MapRepositoryNames { %s }`, strings.Join(queries, ""))

	graphqlResult := make(map[string]*struct {
		DatabaseID int64 `json:"databaseId"`
	})

	if err := client.GraphQL(host, query, nil, &graphqlResult); err != nil {
		return nil, fmt.Errorf("failed to look up repositories: %w", err)
	}

	repoKeys := make([]string, 0, len(repositories))
	for k := range graphqlResult {
		repoKeys = append(repoKeys, k)
	}
	sort.Strings(repoKeys)

	result := make([]int64, len(repositories))
	for i, k := range repoKeys {
		result[i] = graphqlResult[k].DatabaseID
	}
	return result, nil
}
