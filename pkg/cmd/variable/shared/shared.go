package shared

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/iostreams"
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

type VariablePayload struct {
	Name         string  `json:"name,omitempty"`
	Value        string  `json:"value,omitempty"`
	Visibility   string  `json:"visibility,omitempty"`
	Repositories []int64 `json:"selected_repository_ids"`
	IsUpdate     bool    `json:"is_update"`
}

type ListOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	Config     func() (config.Config, error)
	BaseRepo   func() (ghrepo.Interface, error)
	BaseRepoId int64

	OrgName string
	EnvName string
	Page    int
	PerPage int
	Name    string
}

type Variable struct {
	Name             string     `json:"name"`
	Value            string     `json:"value"`
	UpdatedAt        time.Time  `json:"updated_at"`
	Visibility       Visibility `json:"visibility"`
	SelectedReposURL string     `json:"selected_repositories_url"`
	NumSelectedRepos int
}
type VariablesPayload struct {
	Variables []*Variable
}

type Repo struct {
	Name string `json:"name"`
}

type SelectedRepos struct {
	Repositories []*Repo
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

func IsSupportedVariableEntity(entity VariableEntity) bool {
	return entity == Repository || entity == Organization || entity == Environment
}

func GetVariablesForList(opts *ListOptions, getSelectedRepoInfo bool) ([]*Variable, error) {
	client, err := opts.HttpClient()
	if err != nil {
		return nil, fmt.Errorf("could not set http client: %w", err)
	}

	orgName := opts.OrgName
	envName := opts.EnvName

	var baseRepo ghrepo.Interface
	if orgName == "" {
		baseRepo, err = opts.BaseRepo()
		if err != nil {
			return nil, err
		}
	}

	variableEntity, err := GetVariableEntity(orgName, envName)
	if err != nil {
		return nil, err
	}

	if !IsSupportedVariableEntity(variableEntity) {
		return nil, fmt.Errorf("%s variables are not supported", variableEntity)
	}

	var variables []*Variable

	switch variableEntity {
	case Repository:
		variables, err = getRepoVariables(client, baseRepo, opts.Page, opts.PerPage, opts.Name)
	case Environment:
		variables, err = getEnvVariables(client, baseRepo, envName, opts.Page, opts.PerPage, opts.Name, opts.BaseRepoId)
	case Organization:
		var cfg config.Config
		var host string

		cfg, err = opts.Config()
		if err != nil {
			return nil, err
		}

		host, _ = cfg.DefaultHost()
		variables, err = getOrgVariables(client, host, orgName, getSelectedRepoInfo, opts.Page, opts.PerPage, opts.Name)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get variables: %w", err)
	}
	return variables, nil
}

func getOrgVariables(client *http.Client, host, orgName string, getSelectedRepoInfo bool, page, perPage int, name string) ([]*Variable, error) {
	variables, err := getVariables(client, host, fmt.Sprintf(`orgs/%s/actions/variables`, orgName), page, perPage, name)
	if err != nil {
		return nil, err
	}

	if getSelectedRepoInfo {
		err = getSelectedRepositoryInformation(client, variables)
		if err != nil {
			return nil, err
		}
	}
	return variables, nil
}

func getEnvVariables(client *http.Client, repo ghrepo.Interface, envName string, page, perPage int, name string, repoId int64) ([]*Variable, error) {
	path := fmt.Sprintf(`repositories/%d/environments/%s/variables`, repoId, envName)
	return getVariables(client, repo.RepoHost(), path, page, perPage, name)
}

func getRepoVariables(client *http.Client, repo ghrepo.Interface, page, perPage int, name string) ([]*Variable, error) {
	return getVariables(client, repo.RepoHost(), fmt.Sprintf(`repos/%s/actions/variables`,
		ghrepo.FullName(repo)), page, perPage, name)
}

func getVariables(client *http.Client, host, path string, page, perPage int, name string) ([]*Variable, error) {
	var url string
	if name != "" {
		url = fmt.Sprintf("%s%s/%s", ghinstance.RESTPrefix(host), path, name)
		var payload Variable
		err := ApiGet(client, url, &payload)
		if err != nil {
			return nil, err
		}
		return []*Variable{&payload}, nil
	} else {
		url = fmt.Sprintf("%s%s?page=%d&per_page=%d", ghinstance.RESTPrefix(host), path, page, perPage)
		var payload VariablesPayload
		err := ApiGet(client, url, &payload)
		if err != nil {
			return nil, err
		}
		return payload.Variables, nil
	}
}

func ApiGet(client *http.Client, url string, data interface{}) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode > 299 {
		return api.HandleHTTPError(resp)
	}

	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(data); err != nil {
		return err
	}

	return nil
}

func getSelectedRepositoryInformation(client *http.Client, variables []*Variable) error {
	type responseData struct {
		TotalCount int `json:"total_count"`
	}

	for _, variable := range variables {
		if variable.SelectedReposURL == "" {
			continue
		}
		var result responseData
		if err := ApiGet(client, variable.SelectedReposURL, &result); err != nil {
			return fmt.Errorf("failed determining selected repositories for %s: %w", variable.Name, err)
		}
		variable.NumSelectedRepos = result.TotalCount
	}

	return nil
}
