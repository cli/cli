package create

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

func postOrgVariable(client *api.Client, host string, orgName, visibility, variableName, Value string, repositoryIDs []int64, app shared.App) error {
	payload := shared.VariablePayload{
		Name:         variableName,
		Value:        Value,
		Repositories: repositoryIDs,
		Visibility:   visibility,
	}
	path := fmt.Sprintf(`orgs/%s/%s/variables`, orgName, app)

	return postVariable(client, host, path, payload)
}

func postEnvVariable(client *api.Client, repo ghrepo.Interface, envName string, variableName, Value string) error {
	payload := shared.VariablePayload{
		Name:  variableName,
		Value: Value,
	}
	path := fmt.Sprintf(`repos/%s/environments/%s/variables`, ghrepo.FullName(repo), envName)
	return postVariable(client, repo.RepoHost(), path, payload)
}

func postRepoVariable(client *api.Client, repo ghrepo.Interface, variableName, Value string, app shared.App) error {
	payload := shared.VariablePayload{
		Name:  variableName,
		Value: Value,
	}
	path := fmt.Sprintf(`repos/%s/%s/variables`, ghrepo.FullName(repo), app)
	return postVariable(client, repo.RepoHost(), path, payload)
}
