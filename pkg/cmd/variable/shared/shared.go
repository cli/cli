package shared

import (
	"errors"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/safeurl"
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

func (v *Variable) ExportData(fields []string) map[string]any {
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

// SelectedRepositoryCount returns how many repositories the variable is visible to, fetched from the
// given entrusted URL. Callers own reading the URL off the variable and writing the result back.
func SelectedRepositoryCount(apiClient *api.Client, host string, selectedReposURL safeurl.SafeURL) (int, error) {
	response := struct {
		TotalCount int `json:"total_count"`
	}{}
	if err := apiClient.REST(host, "GET", selectedReposURL.String(), nil, &response); err != nil {
		return 0, err
	}
	return response.TotalCount, nil
}
