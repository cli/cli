package set

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
	"github.com/cli/cli/v2/pkg/cmd/variable/shared"
)

const (
	createdOperation = "Created"
	updatedOperation = "Updated"
)

type setPayload struct {
	Name         string  `json:"name,omitempty"`
	Repositories []int64 `json:"selected_repository_ids,omitempty"`
	Value        string  `json:"value,omitempty"`
	Visibility   string  `json:"visibility,omitempty"`
}

type setOptions struct {
	Entity        shared.VariableEntity
	Environment   string
	Key           string
	Organization  string
	Repository    ghrepo.Interface
	RepositoryIDs []int64
	Value         string
	Visibility    string
}

type setResult struct {
	Err       error
	Key       string
	Operation string
}

func setVariable(client *api.Client, host string, opts setOptions) setResult {
	var err error
	var postErr api.HTTPError
	result := setResult{Operation: createdOperation, Key: opts.Key}
	switch opts.Entity {
	case shared.Organization:
		if err = postOrgVariable(client, host, opts.Organization, opts.Visibility, opts.Key, opts.Value, opts.RepositoryIDs); err == nil {
			return result
		} else if errors.As(err, &postErr) && postErr.StatusCode == 409 {
			// Server will return a 409 if variable already exists
			result.Operation = updatedOperation
			err = patchOrgVariable(client, host, opts.Organization, opts.Visibility, opts.Key, opts.Value, opts.RepositoryIDs)
		}
	case shared.Environment:
		var ids []int64
		ids, err = api.GetRepoIDs(client, opts.Repository.RepoHost(), []ghrepo.Interface{opts.Repository})
		if err != nil || len(ids) != 1 {
			err = fmt.Errorf("failed to look up repository %s: %w", ghrepo.FullName(opts.Repository), err)
			break
		}
		if err = postEnvVariable(client, opts.Repository.RepoHost(), ids[0], opts.Environment, opts.Key, opts.Value); err == nil {
			return result
		} else if errors.As(err, &postErr) && postErr.StatusCode == 409 {
			// Server will return a 409 if variable already exists
			result.Operation = updatedOperation
			err = patchEnvVariable(client, opts.Repository.RepoHost(), ids[0], opts.Environment, opts.Key, opts.Value)
		}
	default:
		if err = postRepoVariable(client, opts.Repository, opts.Key, opts.Value); err == nil {
			return result
		} else if errors.As(err, &postErr) && postErr.StatusCode == 409 {
			// Server will return a 409 if variable already exists
			result.Operation = updatedOperation
			err = patchRepoVariable(client, opts.Repository, opts.Key, opts.Value)
		}
	}
	if err != nil {
		result.Err = fmt.Errorf("failed to set variable %q: %w", opts.Key, err)
	}
	return result
}

func postVariable(client *api.Client, host string, path safeurl.SafeURL, payload any) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to serialize: %w", err)
	}
	requestBody := bytes.NewReader(payloadBytes)
	return client.REST(host, "POST", path.String(), requestBody, nil)
}

func postOrgVariable(client *api.Client, host, orgName, visibility, variableName, value string, repositoryIDs []int64) error {
	payload := setPayload{
		Name:         variableName,
		Value:        value,
		Visibility:   visibility,
		Repositories: repositoryIDs,
	}
	path, err := safeurl.JoinPath("orgs", orgName, "actions", "variables")
	if err != nil {
		return err
	}
	return postVariable(client, host, path, payload)
}

func postEnvVariable(client *api.Client, host string, repoID int64, envName, variableName, value string) error {
	payload := setPayload{
		Name:  variableName,
		Value: value,
	}
	path, err := safeurl.JoinPath("repositories", strconv.FormatInt(repoID, 10), "environments", envName, "variables")
	if err != nil {
		return err
	}
	return postVariable(client, host, path, payload)
}

func postRepoVariable(client *api.Client, repo ghrepo.Interface, variableName, value string) error {
	payload := setPayload{
		Name:  variableName,
		Value: value,
	}
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "actions", "variables")
	if err != nil {
		return err
	}
	return postVariable(client, repo.RepoHost(), path, payload)
}

func patchVariable(client *api.Client, host string, path safeurl.SafeURL, payload any) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to serialize: %w", err)
	}
	requestBody := bytes.NewReader(payloadBytes)
	return client.REST(host, "PATCH", path.String(), requestBody, nil)
}

func patchOrgVariable(client *api.Client, host, orgName, visibility, variableName, value string, repositoryIDs []int64) error {
	payload := setPayload{
		Value:        value,
		Visibility:   visibility,
		Repositories: repositoryIDs,
	}
	path, err := safeurl.JoinPath("orgs", orgName, "actions", "variables", variableName)
	if err != nil {
		return err
	}
	return patchVariable(client, host, path, payload)
}

func patchEnvVariable(client *api.Client, host string, repoID int64, envName, variableName, value string) error {
	payload := setPayload{
		Value: value,
	}
	path, err := safeurl.JoinPath("repositories", strconv.FormatInt(repoID, 10), "environments", envName, "variables", variableName)
	if err != nil {
		return err
	}
	return patchVariable(client, host, path, payload)
}

func patchRepoVariable(client *api.Client, repo ghrepo.Interface, variableName, value string) error {
	payload := setPayload{
		Value: value,
	}
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "actions", "variables", variableName)
	if err != nil {
		return err
	}
	return patchVariable(client, repo.RepoHost(), path, payload)
}
