package set

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghrepo"
)

type SecretPayload struct {
	EncryptedValue string `json:"encrypted_value"`
	Visibility     string `json:"visibility,omitempty"`
	Repositories   []int  `json:"selected_repository_ids,omitempty"`
	KeyID          string `json:"key_id"`
}

type PubKey struct {
	Raw [32]byte
	ID  string `json:"key_id"`
	Key string
}

func getPK(client *api.Client, host, path string) (*PubKey, error) {
	pk := PubKey{}
	err := client.REST(host, "GET", path, nil, &pk)
	if err != nil {
		return nil, err
	}

	if pk.Key == "" {
		return nil, fmt.Errorf("failed to find public key at %s/%s", host, path)
	}

	decoded, err := base64.StdEncoding.DecodeString(pk.Key)
	if err != nil {
		return nil, fmt.Errorf("failed to decode public key: %w", err)
	}

	copy(pk.Raw[:], decoded[0:32])
	return &pk, nil
}

func getOrgPublicKey(client *api.Client, host, orgName string) (*PubKey, error) {
	return getPK(client, host, fmt.Sprintf("orgs/%s/actions/secrets/public-key", orgName))
}

func getRepoPubKey(client *api.Client, repo ghrepo.Interface) (*PubKey, error) {
	return getPK(client, repo.RepoHost(), fmt.Sprintf("repos/%s/actions/secrets/public-key",
		ghrepo.FullName(repo)))
}

func putSecret(client *api.Client, host, path string, payload SecretPayload) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to serialize: %w", err)
	}
	requestBody := bytes.NewReader(payloadBytes)

	return client.REST(host, "PUT", path, requestBody, nil)
}

func putOrgSecret(client *api.Client, pk *PubKey, host, eValue string, orgRepos []api.OrgRepo, opts SetOptions) error {
	secretName := opts.SecretName
	orgName := opts.OrgName
	visibility := opts.Visibility

	repositoryIDs := []int{}
	for _, orgRepo := range orgRepos {
		repositoryIDs = append(repositoryIDs, orgRepo.DatabaseID)
	}

	payload := SecretPayload{
		EncryptedValue: eValue,
		KeyID:          pk.ID,
		Repositories:   repositoryIDs,
		Visibility:     visibility,
	}
	path := fmt.Sprintf("orgs/%s/actions/secrets/%s", orgName, secretName)

	return putSecret(client, host, path, payload)
}

func putRepoSecret(client *api.Client, pk *PubKey, repo ghrepo.Interface, eValue string, opts SetOptions) error {
	payload := SecretPayload{
		EncryptedValue: eValue,
		KeyID:          pk.ID,
	}
	path := fmt.Sprintf("repos/%s/actions/secrets/%s", ghrepo.FullName(repo), opts.SecretName)
	return putSecret(client, repo.RepoHost(), path, payload)
}

func mapRepoNamesToID(client *api.Client, host, orgName string, repositoryNames []string) ([]api.OrgRepo, error) {
	queries := make([]string, 0, len(repositoryNames))
	for _, repoName := range repositoryNames {
		queries = append(queries, fmt.Sprintf(`
			%s: repository(owner: %q, name :%q) {
			  name
				databaseId
			}
		`, repoName, orgName, repoName))
	}

	query := fmt.Sprintf(`query MapRepositoryNames { %s }`, strings.Join(queries, ""))

	graphqlResult := make(map[string]*api.OrgRepo)

	err := client.GraphQL(host, query, nil, &graphqlResult)

	gqlErr, isGqlErr := err.(*api.GraphQLErrorResponse)
	if isGqlErr {
		for _, ge := range gqlErr.Errors {
			if ge.Type == "NOT_FOUND" {
				return nil, fmt.Errorf("could not find %s/%s", orgName, ge.Path[0])
			}
		}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to look up repositories: %w", err)
	}

	result := make([]api.OrgRepo, 0, len(repositoryNames))

	for _, repoName := range repositoryNames {
		result = append(result, *graphqlResult[repoName])
	}

	return result, nil
}
