package shared

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/iostreams"
)

type Visibility string

const (
	All      = "all"
	Private  = "private"
	Selected = "selected"
)

type App string

const (
	Actions = "actions"
	Unknown = "unknown"
)

func (app App) String() string {
	return string(app)
}

func (app App) Title() string {
	return text.Title(app.String())
}

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
	Repositories []int64 `json:"selected_repository_ids,omitempty"`
}


type repoNamesResult struct {
	Ids []int64
	Err error
}

type PostPatchOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	Config     func() (config.Config, error)
	BaseRepo   func() (ghrepo.Interface, error)

	VariableName    string
	OrgName         string
	EnvName         string
	Body            string
	Visibility      string
	RepositoryNames []string
	Prompter        iprompter
}

type ListOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	Config     func() (config.Config, error)
	BaseRepo   func() (ghrepo.Interface, error)

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
	Name string `json: "name"`
}

type SelectedRepos struct {
	Repositories []*Repo
}

type httpClient interface {
	Do(*http.Request) (*http.Response, error)
}

type iprompter interface {
	Input(string, string) (string, error)
	Select(string, string, []string) (int, error)
	Confirm(string, bool) (bool, error)
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

func IsSupportedVariableEntity(app App, entity VariableEntity) bool {
	switch app {
	case Actions:
		return entity == Repository || entity == Organization || entity == Environment
	default:
		return false
	}
}

// This does similar logic to `api.RepoNetwork`, but without the overfetching.
func MapRepoToID(client *api.Client, host string, repositories []ghrepo.Interface) ([]int64, error) {
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

func getCurrentSelectedRepos(client httpClient, url string) string {
	var selectedRepositories SelectedRepos
	err := apiGet(client, url, &selectedRepositories)
	if err != nil {
		return ""
	}
	names := make([]string, 0)
	for _, repo := range selectedRepositories.Repositories {
		names = append(names, repo.Name)
	}
	return strings.Join(names, ",")
}

func MapRepoNamesToIDs(client *api.Client, host, defaultOwner string, repositoryNames []string) ([]int64, error) {
	repos := make([]ghrepo.Interface, 0, len(repositoryNames))
	for _, repositoryName := range repositoryNames {
		var repo ghrepo.Interface
		if strings.Contains(repositoryName, "/") || defaultOwner == "" {
			var err error
			repo, err = ghrepo.FromFullNameWithHost(repositoryName, host)
			if err != nil {
				return nil, fmt.Errorf("invalid repository name")
			}
		} else {
			repo = ghrepo.NewWithHost(defaultOwner, repositoryName, host)
		}
		repos = append(repos, repo)
	}
	repositoryIDs, err := MapRepoToID(client, host, repos)
	if err != nil {
		return nil, fmt.Errorf("failed to look up IDs for repositories %v: %w", repositoryNames, err)
	}
	return repositoryIDs, nil
}

func GetRepoIds(client *api.Client, host, orgName string, repoNames []string) repoNamesResult {
	if len(repoNames) == 0 {
		return repoNamesResult{}
	}
	repositoryIDs, err := MapRepoNamesToIDs(client, host, orgName, repoNames)
	return repoNamesResult{
		Ids: repositoryIDs,
		Err: err,
	}
}

func GetVariablesFromOptions(opts *PostPatchOptions, client *api.Client, host string, isUpdate bool) (map[string]VariablePayload, error) {
	variables := make(map[string]VariablePayload)
	values, err := getBody(opts, client, host, isUpdate)
	if err != nil {
		return nil, fmt.Errorf("did not understand variable body: %w", err)
	}
	variables[opts.VariableName] = values
	return variables, nil
}

func getVarFromRow(opts *PostPatchOptions, client *api.Client, host string, row []string, isUpdate bool) (VariablePayload, error) {
	rowLength := len(row)
	index := 1
	if rowLength < 2 {
		return VariablePayload{}, fmt.Errorf("less than 2 records in a row in file")
	}
	if rowLength > 3 && opts.OrgName == "" {
		return VariablePayload{}, fmt.Errorf("more than 3 vals in a row in file for non org variable %s", row[0])
	}

	var variable VariablePayload
	variable.Value = row[index]
	index++
	if isUpdate {
		if rowLength >= 3 && len(row[index]) > 0 {
			variable.Name = row[index]
		} else {
			variable.Name = row[0]
		}
		index++
	} else {
		variable.Name = row[0]
	}
	if opts.OrgName != "" {
		if !isUpdate && rowLength >= 3 {
			variable.Visibility = row[index]
		} else if rowLength >= 4 {
			variable.Visibility = row[index]
		}
		index++
		if variable.Visibility == Selected {
			if rowLength < 5 {
				// do not exit here as repositoryNames in opts may have the info.
				log.Printf("selected visibility with no repos mentioned for variable %s", row[0])
			}
			repoIdRes := GetRepoIds(client, host, opts.OrgName, row[index:])
			if repoIdRes.Err != nil {
				return VariablePayload{}, repoIdRes.Err
			}
			variable.Repositories = repoIdRes.Ids
		}
	}
	return variable, nil
}

func getBody(opts *PostPatchOptions, client *api.Client, host string, isUpdate bool) (VariablePayload, error) {
	if opts.Body != "" {
		return getVarFromRow(opts, client, host, strings.Split(opts.VariableName+","+string(opts.Body), ","), isUpdate)
	}

	values := make([]string, 0)
	if opts.IO.CanPrompt() {
		var currentVar Variable
		values = append(values, opts.VariableName)
		data, err := opts.Prompter.Input("Value", currentVar.Value)
		if err != nil {
			return VariablePayload{}, err
		}
		values = append(values, data)
		if isUpdate {
			data, err = opts.Prompter.Input("New name", currentVar.Name)
			if err != nil {
				return VariablePayload{}, err
			}
			values = append(values, data)
		}

		if opts.OrgName != "" {
			data, err = opts.Prompter.Input("Visibility", string(currentVar.Visibility))
			if err != nil {
				return VariablePayload{}, err
			}
			values = append(values, data)

			if data == Selected {
				data, err = opts.Prompter.Input("Repos(comma separated)", getCurrentSelectedRepos(client.HTTP(), currentVar.SelectedReposURL))
				if err != nil {
					return VariablePayload{}, err
				}
				values = append(values, strings.Split(data, ",")...)
			}
		}

		fmt.Fprintln(opts.IO.Out)
		return getVarFromRow(opts, client, host, values, isUpdate)
	}

	body, err := io.ReadAll(opts.IO.In)
	if err != nil {
		return VariablePayload{}, fmt.Errorf("failed to read from standard input: %w", err)
	}

	return getVarFromRow(opts, client, host, strings.Split(opts.VariableName+","+string(bytes.TrimRight(body, "\r\n")), ","), isUpdate)
}


func apiGet(client httpClient, url string, data interface{}) error {
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

func getSelectedRepositoryInformation(client httpClient, variables []*Variable) error {
	type responseData struct {
		TotalCount int `json:"total_count"`
	}

	for _, variable := range variables {
		if variable.SelectedReposURL == "" {
			continue
		}
		var result responseData
		if err := apiGet(client, variable.SelectedReposURL, &result); err != nil {
			return fmt.Errorf("failed determining selected repositories for %s: %w", variable.Name, err)
		}
		variable.NumSelectedRepos = result.TotalCount
	}

	return nil
}
