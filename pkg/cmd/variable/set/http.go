package set

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/githubrest"
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

// setVariable creates or updates a variable. It takes both a REST client, which
// performs the create and update requests, and a GraphQL client, which resolves
// a repository name to the ID that the environment variable endpoint requires.
func setVariable(ctx context.Context, client *githubrest.Client, gqlClient *api.Client, opts setOptions) setResult {
	var err error
	var postErr *githubrest.ErrorResponse
	result := setResult{Operation: createdOperation, Key: opts.Key}
	switch opts.Entity {
	case shared.Organization:
		if err = postOrgVariable(ctx, client, opts.Organization, opts.Visibility, opts.Key, opts.Value, opts.RepositoryIDs); err == nil {
			return result
		} else if errors.As(err, &postErr) && postErr.StatusCode == 409 {
			// Server will return a 409 if variable already exists
			result.Operation = updatedOperation
			err = patchOrgVariable(ctx, client, opts.Organization, opts.Visibility, opts.Key, opts.Value, opts.RepositoryIDs)
		}
	case shared.Environment:
		var ids []int64
		ids, err = api.GetRepoIDs(gqlClient, opts.Repository.RepoHost(), []ghrepo.Interface{opts.Repository})
		if err != nil || len(ids) != 1 {
			err = fmt.Errorf("failed to look up repository %s: %w", ghrepo.FullName(opts.Repository), err)
			break
		}
		if err = postEnvVariable(ctx, client, ids[0], opts.Environment, opts.Key, opts.Value); err == nil {
			return result
		} else if errors.As(err, &postErr) && postErr.StatusCode == 409 {
			// Server will return a 409 if variable already exists
			result.Operation = updatedOperation
			err = patchEnvVariable(ctx, client, ids[0], opts.Environment, opts.Key, opts.Value)
		}
	default:
		if err = postRepoVariable(ctx, client, opts.Repository, opts.Key, opts.Value); err == nil {
			return result
		} else if errors.As(err, &postErr) && postErr.StatusCode == 409 {
			// Server will return a 409 if variable already exists
			result.Operation = updatedOperation
			err = patchRepoVariable(ctx, client, opts.Repository, opts.Key, opts.Value)
		}
	}
	if err != nil {
		result.Err = fmt.Errorf("failed to set variable %q: %w", opts.Key, err)
	}
	return result
}

func postVariable(ctx context.Context, client *githubrest.Client, path safeurl.SafeURL, payload interface{}) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to serialize: %w", err)
	}
	req, err := client.NewRequest(ctx, http.MethodPost, path.String(), bytes.NewReader(payloadBytes))
	if err != nil {
		return err
	}
	_, err = client.Do(req, nil)
	return err
}

func postOrgVariable(ctx context.Context, client *githubrest.Client, orgName, visibility, variableName, value string, repositoryIDs []int64) error {
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
	return postVariable(ctx, client, path, payload)
}

func postEnvVariable(ctx context.Context, client *githubrest.Client, repoID int64, envName, variableName, value string) error {
	payload := setPayload{
		Name:  variableName,
		Value: value,
	}
	path, err := safeurl.JoinPath("repositories", strconv.FormatInt(repoID, 10), "environments", envName, "variables")
	if err != nil {
		return err
	}
	return postVariable(ctx, client, path, payload)
}

func postRepoVariable(ctx context.Context, client *githubrest.Client, repo ghrepo.Interface, variableName, value string) error {
	payload := setPayload{
		Name:  variableName,
		Value: value,
	}
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "actions", "variables")
	if err != nil {
		return err
	}
	return postVariable(ctx, client, path, payload)
}

func patchVariable(ctx context.Context, client *githubrest.Client, path safeurl.SafeURL, payload interface{}) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to serialize: %w", err)
	}
	req, err := client.NewRequest(ctx, http.MethodPatch, path.String(), bytes.NewReader(payloadBytes))
	if err != nil {
		return err
	}
	_, err = client.Do(req, nil)
	return err
}

func patchOrgVariable(ctx context.Context, client *githubrest.Client, orgName, visibility, variableName, value string, repositoryIDs []int64) error {
	payload := setPayload{
		Value:        value,
		Visibility:   visibility,
		Repositories: repositoryIDs,
	}
	path, err := safeurl.JoinPath("orgs", orgName, "actions", "variables", variableName)
	if err != nil {
		return err
	}
	return patchVariable(ctx, client, path, payload)
}

func patchEnvVariable(ctx context.Context, client *githubrest.Client, repoID int64, envName, variableName, value string) error {
	payload := setPayload{
		Value: value,
	}
	path, err := safeurl.JoinPath("repositories", strconv.FormatInt(repoID, 10), "environments", envName, "variables", variableName)
	if err != nil {
		return err
	}
	return patchVariable(ctx, client, path, payload)
}

func patchRepoVariable(ctx context.Context, client *githubrest.Client, repo ghrepo.Interface, variableName, value string) error {
	payload := setPayload{
		Value: value,
	}
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "actions", "variables", variableName)
	if err != nil {
		return err
	}
	return patchVariable(ctx, client, path, payload)
}
