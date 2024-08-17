package shared

import (
	"errors"
	"fmt"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/pkg/cmdutil"
)

type Visibility string

const (
	All      = "all"
	Private  = "private"
	Selected = "selected"
)

type VariableEntity string

const (
	Repository   = "repository"
	Organization = "organization"
	Environment  = "environment"
)

type Variable struct {
	Name             string     `json:"name"`
	Value            string     `json:"value"`
	UpdatedAt        time.Time  `json:"updated_at"`
	CreatedAt        time.Time  `json:"created_at"`
	Visibility       Visibility `json:"visibility"`
	SelectedReposURL string     `json:"selected_repositories_url"`
	NumSelectedRepos int        `json:"num_selected_repos"`
}

var VariableJSONFields = []string{
	"name",
	"value",
	"visibility",
	"updatedAt",
	"createdAt",
	"numSelectedRepos",
	"selectedReposURL",
}

func (v *Variable) ExportData(fields []string) map[string]interface{} {
	return cmdutil.StructExportData(v, fields)
}

func GetVariableEntity(orgName, envName string) (VariableEntity, error) {
	orgSet := orgName != ""
	envSet := envName != ""

	if orgSet && envSet {
		return "", errors.New("cannot specify multiple variable entities")
	}

	if orgSet {
		return Organization, nil
	}
	if envSet {
		return Environment, nil
	}
	return Repository, nil
}

func PopulateMultipleSelectedRepositoryInformation(apiClient *api.Client, host string, variables []Variable) error {
	for i, variable := range variables {
		if err := PopulateSelectedRepositoryInformation(apiClient, host, &variable); err != nil {
			return err
		}
		variables[i] = variable
	}
	return nil
}

func PopulateSelectedRepositoryInformation(apiClient *api.Client, host string, variable *Variable) error {
	if variable.SelectedReposURL == "" {
		return nil
	}

	response := struct {
		TotalCount int `json:"total_count"`
	}{}
	if err := apiClient.REST(host, "GET", variable.SelectedReposURL, nil, &response); err != nil {
		return fmt.Errorf("failed determining selected repositories for %s: %w", variable.Name, err)
	}
	variable.NumSelectedRepos = response.TotalCount
	return nil
}
