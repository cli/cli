package list

import (
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/env/shared"
)

type EnvironmentLister struct {
	HTTPClient *http.Client
}

func (e *EnvironmentLister) List(repo ghrepo.Interface, limit int, isTTY bool) ([]shared.Environment, error) {
	path := fmt.Sprintf("repos/%s/environments", ghrepo.FullName(repo))

	perPage := 100
	if limit > 0 && limit < 100 {
		perPage = limit
	}
	path += fmt.Sprintf("?per_page=%d", perPage)

	client := api.NewClientFromHTTP(e.HTTPClient)

	var environments []shared.Environment
pagination:
	for path != "" {
		var response shared.EnvironmentPayload
		var err error
		path, err = client.RESTWithNext(repo.RepoHost(), "GET", path, nil, &response)
		if err != nil {
			return nil, err
		}

		environments = append(environments, response.Environments...)

		if limit > 0 && len(environments) >= limit {
			environments = environments[:limit]
			break pagination
		}
	}

	if isTTY {
		for idx, environment := range environments {
			secretsTotalCount, err := getSecretsTotalCount(client, repo, environment.Name)
			if err != nil {
				return nil, err
			}
			variablesTotalCount, err := getVariablesTotalCount(client, repo, environment.Name)
			if err != nil {
				return nil, err
			}
			environments[idx].SecretsTotalCount = secretsTotalCount
			environments[idx].VariablesTotalCount = variablesTotalCount
		}
	}

	return environments, nil
}

func getSecretsTotalCount(client *api.Client, repo ghrepo.Interface, environment string) (int, error) {
	path := fmt.Sprintf("repos/%s/environments/%s/secrets?per_page=1", ghrepo.FullName(repo), environment)

	var response struct {
		TotalCount int `json:"total_count"`
	}

	err := client.REST(repo.RepoHost(), "GET", path, nil, &response)
	if err != nil {
		return 0, fmt.Errorf("could not get secrets total count for %s environment: %w", environment, err)
	}

	return response.TotalCount, nil
}

func getVariablesTotalCount(client *api.Client, repo ghrepo.Interface, environment string) (int, error) {
	path := fmt.Sprintf("repos/%s/environments/%s/variables?per_page=1", ghrepo.FullName(repo), environment)

	var response struct {
		TotalCount int `json:"total_count"`
	}

	err := client.REST(repo.RepoHost(), "GET", path, nil, &response)
	if err != nil {
		return 0, fmt.Errorf("could not get variables total count for %s environment: %w", environment, err)
	}

	return response.TotalCount, nil
}
