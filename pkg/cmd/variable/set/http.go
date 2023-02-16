package set

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/variable/shared"
)

func postVariable(client *api.Client, host, path string, payload interface{}) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to serialize: %w", err)
	}
	requestBody := bytes.NewReader(payloadBytes)

	return client.REST(host, "POST", path, requestBody, nil)
}

func postOrgVariable(client *api.Client, host string, orgName, visibility, variableName, Value string, repositoryIDs []int64) error {
	payload := shared.VariablePayload{
		Name:         variableName,
		Value:        Value,
		Repositories: repositoryIDs,
		Visibility:   visibility,
	}
	path := fmt.Sprintf(`orgs/%s/%s/variables`, orgName, "actions")

	return postVariable(client, host, path, payload)
}

func postEnvVariable(client *api.Client, repo ghrepo.Interface, envName string, variableName, Value string) error {
	payload := shared.VariablePayload{
		Name:  variableName,
		Value: Value,
	}
	path := fmt.Sprintf(`repositories/%s/environments/%s/variables`, ghrepo.FullName(repo), envName)
	return postVariable(client, repo.RepoHost(), path, payload)
}

func postRepoVariable(client *api.Client, repo ghrepo.Interface, variableName, Value string) error {
	payload := shared.VariablePayload{
		Name:  variableName,
		Value: Value,
	}
	path := fmt.Sprintf(`repositories/%s/%s/variables`, ghrepo.FullName(repo), "actions")
	return postVariable(client, repo.RepoHost(), path, payload)
}

func patchVariable(client *api.Client, host, path string, payload interface{}) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to serialize: %w", err)
	}
	requestBody := bytes.NewReader(payloadBytes)

	return client.REST(host, "PATCH", path, requestBody, nil)
}

func patchOrgVariable(client *api.Client, host string, orgName, visibility, variableName, updatedVariableName, value string, repositoryIDs []int64) error {
	payload := shared.VariablePayload{
		Name:         updatedVariableName,
		Value:        value,
		Repositories: repositoryIDs,
		Visibility:   visibility,
	}
	path := fmt.Sprintf(`orgs/%s/%s/variables/%s`, orgName, "actions", variableName)

	return patchVariable(client, host, path, payload)
}

func patchEnvVariable(client *api.Client, repo ghrepo.Interface, envName string, variableName, updatedVariableName, value string) error {
	payload := shared.VariablePayload{
		Name:  updatedVariableName,
		Value: value,
	}
	path := fmt.Sprintf(`repos/%s/environments/%s/variables/%s`, ghrepo.FullName(repo), envName, variableName)
	return patchVariable(client, repo.RepoHost(), path, payload)
}

func patchRepoVariable(client *api.Client, repo ghrepo.Interface, variableName, updatedVariableName, value string) error {
	payload := shared.VariablePayload{
		Name:  updatedVariableName,
		Value: value,
	}
	path := fmt.Sprintf(`repos/%s/%s/variables/%s`, ghrepo.FullName(repo), "actions", variableName)
	return patchVariable(client, repo.RepoHost(), path, payload)
}
