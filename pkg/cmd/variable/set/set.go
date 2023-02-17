package set

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/variable/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/hashicorp/go-multierror"
	"github.com/spf13/cobra"
)

type iprompter interface {
	Input(string, string) (string, error)
	Select(string, string, []string) (int, error)
	Confirm(string, bool) (bool, error)
}

type SetOptions struct {
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
	CsvFile         string
	Prompter        iprompter
}

type repoNamesResult struct {
	Ids []int64
	Err error
}

func NewCmdSet(f *cmdutil.Factory, runF func(*SetOptions) error) *cobra.Command {
	opts := &SetOptions{
		IO:         f.IOStreams,
		Config:     f.Config,
		HttpClient: f.HttpClient,
		Prompter:   f.Prompter,
	}

	cmd := &cobra.Command{
		Use:   "set <variable-name>",
		Short: "Set variables",
		Long: heredoc.Doc(`
			Set a value for a variable on one of the following levels:
			- repository (default): available to Actions runs in a repository
			- environment: available to Actions runs for a deployment environment in a repository
			- organization: available to Actions runs within an organization

			Organization variable can optionally be restricted to only be available to
			specific repositories.
		`),
		Example: heredoc.Doc(`
			# Add variable value for the current repository in an interactive prompt
			$ gh variable set MYVARIABLE

			# Read variable value from an environment variable
			$ gh variable set MYVARIABLE --body "$ENV_VALUE"

			# Read variable value from a file
			$ gh variable set MYVARIABLE < myfile.txt

			# Set variable for a deployment environment in the current repository
			$ gh variable set MYVARIABLE --env myenvironment

			# Set organization-level variable visible to both public and private repositories
			$ gh variable set MYVARIABLE --org myOrg --visibility all

			# Set organization-level variable visible to specific repositories
			$ gh variable set MYVARIABLE --org myOrg --repos repo1,repo2,repo3

			# Set multiple variables from the csv in the prescribed format(name, value, visibility(if org), repos(if visibility = selected))
			$ gh variable set -f file.csv

		`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if err := cmdutil.MutuallyExclusive("specify only one of `--org` or `--env`", opts.OrgName != "", opts.EnvName != ""); err != nil {
				return err
			}

			if err := cmdutil.MutuallyExclusive("specify only one of `--body` or `--csv-file`", opts.Body != "", opts.CsvFile != ""); err != nil {
				return err
			}

			if len(args) == 0 {
				if opts.CsvFile == "" {
					return cmdutil.FlagErrorf("must pass name argument")
				}
			} else {
				opts.VariableName = args[0]
			}

			if cmd.Flags().Changed("visibility") {
				if opts.OrgName == "" {
					return cmdutil.FlagErrorf("`--visibility` is only supported with `--org`")
				}

				if opts.Visibility != shared.Selected && len(opts.RepositoryNames) > 0 {
					return cmdutil.FlagErrorf("`--repos` is only supported with `--visibility=selected`")
				}

				if opts.Visibility == shared.Selected && len(opts.RepositoryNames) == 0 {
					return cmdutil.FlagErrorf("`--repos` list required with `--visibility=selected`")
				}
			} else {
				if len(opts.RepositoryNames) > 0 {
					opts.Visibility = shared.Selected
				}
			}

			if runF != nil {
				return runF(opts)
			}

			return setRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.OrgName, "org", "o", "", "Set `organization` variable")
	cmd.Flags().StringVarP(&opts.EnvName, "env", "e", "", "Set deployment `environment` variable")
	cmdutil.StringEnumFlag(cmd, &opts.Visibility, "visibility", "v", shared.Private, []string{shared.All, shared.Private, shared.Selected}, "Set visibility for an organization variable")
	cmd.Flags().StringSliceVarP(&opts.RepositoryNames, "repos", "r", []string{}, "List of `repositories` that can access an organization variable")
	cmd.Flags().StringVarP(&opts.Body, "body", "b", "", "The value for the variable (reads from standard input if not specified)")
	cmd.Flags().StringVarP(&opts.CsvFile, "csv-file", "f", "", "Load variable names and values from a csv-formatted `file`")

	return cmd
}

func setRun(opts *SetOptions) error {
	c, err := opts.HttpClient()
	if err != nil {
		return fmt.Errorf("could not set http client: %w", err)
	}
	client := api.NewClientFromHTTP(c)
	orgName := opts.OrgName
	envName := opts.EnvName

	var host string
	var baseRepo ghrepo.Interface
	if orgName == "" {
		baseRepo, err = opts.BaseRepo()
		if err != nil {
			return err
		}
		host = baseRepo.RepoHost()
	} else {
		cfg, err := opts.Config()
		if err != nil {
			return err
		}
		host, _ = cfg.DefaultHost()
	}
	variables, err := getVariablesFromOptions(opts, client, host)
	if err != nil {
		return err
	}

	variableEntity, err := shared.GetVariableEntity(orgName, envName)
	if err != nil {
		return err
	}

	if !shared.IsSupportedVariableEntity(variableEntity) {
		return fmt.Errorf("%s variables are not supported", variableEntity)
	}

	repoIdRes := getRepoIds(client, host, opts.OrgName, opts.RepositoryNames)
	if repoIdRes.Err != nil {
		return repoIdRes.Err
	}

	setc := make(chan setResult)
	for variableKey, variable := range variables {
		go func(variableKey string, variable shared.VariablePayload) {
			var repoIds []int64
			var visibility string
			visibility = opts.Visibility
			//cmd line opt overrides file
			if len(repoIdRes.Ids) > 0 {
				repoIds = repoIdRes.Ids
			} else if variable.Visibility != "" {
				visibility = variable.Visibility
				repoIds = variable.Repositories
			}
			setc <- setVariable(opts, host, client, baseRepo, variableKey, variable.Value, visibility, repoIds, variableEntity, variable.IsUpdate)
		}(variableKey, variable)
	}

	err = nil
	cs := opts.IO.ColorScheme()
	for i := 0; i < len(variables); i++ {
		result := <-setc
		if result.err != nil {
			err = multierror.Append(err, result.err)
			continue
		}
		if result.value != "" {
			fmt.Fprintln(opts.IO.Out, result.value)
			continue
		}
		if !opts.IO.IsStdoutTTY() {
			continue
		}
		target := orgName
		if orgName == "" {
			target = ghrepo.FullName(baseRepo)
		}
		fmt.Fprintf(opts.IO.Out, "%s Set %s variable %s for %s\n", cs.SuccessIcon(), "actions", result.key, target)
	}
	return err
}

type setResult struct {
	key   string
	value string
	err   error
}

func setVariable(opts *SetOptions, host string, client *api.Client, baseRepo ghrepo.Interface, variableKey, variableValue, visibility string, repositoryIDs []int64, entity shared.VariableEntity, isUpdate bool) (res setResult) {
	orgName := opts.OrgName
	envName := opts.EnvName
	res.key = variableKey
	var err error
	switch entity {
	case shared.Organization:
		if !isUpdate {
			err = postOrgVariable(client, host, orgName, visibility, variableKey, variableValue, repositoryIDs)
		} else {
			err = patchOrgVariable(client, host, orgName, visibility, variableKey, variableKey, variableValue, repositoryIDs)
		}
	case shared.Environment:
		if !isUpdate {
			err = postEnvVariable(client, baseRepo, envName, variableKey, variableValue)
		} else {
			err = patchEnvVariable(client, baseRepo, envName, variableKey, variableKey, variableValue)
		}
	default:
		if !isUpdate {
			err = postRepoVariable(client, baseRepo, variableKey, variableValue)
		} else {
			err = patchRepoVariable(client, baseRepo, variableKey, variableKey, variableValue)
		}
	}
	if err != nil {
		res.err = fmt.Errorf("failed to set variable %q: %w", variableKey, err)
		return
	}
	return
}

func getVariablesFromOptions(opts *SetOptions, client *api.Client, host string) (map[string]shared.VariablePayload, error) {
	variables := make(map[string]shared.VariablePayload)
	if opts.CsvFile != "" {
		var r io.Reader
		if opts.CsvFile == "-" {
			defer opts.IO.In.Close()
			r = opts.IO.In
		} else {
			f, err := os.Open(opts.CsvFile)
			if err != nil {
				return nil, fmt.Errorf("failed to open env file: %w", err)
			}
			defer f.Close()
			r = f
		}
		csvReader := csv.NewReader(r)
		csvReader.Comment = '#'
		csvReader.FieldsPerRecord = -1
		csvReader.TrimLeadingSpace = true
		records, err := csvReader.ReadAll()
		if err != nil {
			return nil, fmt.Errorf("error parsing csv file: %w", err)
		}
		if len(records) == 0 {
			return nil, fmt.Errorf("no variables found in file")
		}
		for _, row := range records {
			variable, err := getVarFromRow(opts, client, host, row)
			if err != nil {
				return nil, err
			}
			currentVariablePtr, _ := shared.GetVariablesForList(&shared.ListOptions{HttpClient: opts.HttpClient,
				IO:       opts.IO,
				Config:   opts.Config,
				BaseRepo: opts.BaseRepo,
				OrgName:  opts.OrgName,
				EnvName:  opts.EnvName,
				Page:     0,
				PerPage:  0,
				Name:     variable.Name,
			}, true)
			variable.IsUpdate = currentVariablePtr != nil
			variables[row[0]] = variable
		}
		return variables, nil
	}

	currentVariablePtr, _ := shared.GetVariablesForList(&shared.ListOptions{HttpClient: opts.HttpClient,
		IO:       opts.IO,
		Config:   opts.Config,
		BaseRepo: opts.BaseRepo,
		OrgName:  opts.OrgName,
		EnvName:  opts.EnvName,
		Page:     0,
		PerPage:  0,
		Name:     opts.VariableName,
	}, true)

	values, err := getBody(opts, client, host, currentVariablePtr)
	values.IsUpdate = currentVariablePtr != nil
	if err != nil {
		return nil, fmt.Errorf("did not understand variable body: %w", err)
	}
	variables[opts.VariableName] = values
	return variables, nil
}

func getVarFromRow(opts *SetOptions, client *api.Client, host string, row []string) (shared.VariablePayload, error) {
	rowLength := len(row)
	index := 1
	if rowLength < 2 {
		return shared.VariablePayload{}, fmt.Errorf("less than 2 records in a row in file")
	}
	if rowLength > 3 && opts.OrgName == "" {
		return shared.VariablePayload{}, fmt.Errorf("more than 3 vals in a row in file for non org variable %s", row[0])
	}

	var variable shared.VariablePayload
	variable.Value = row[index]
	index++
	variable.Name = row[0]
	if opts.OrgName != "" {
		if rowLength >= 3 {
			variable.Visibility = row[index]
		}
		index++
		if variable.Visibility == shared.Selected {
			if rowLength < 5 {
				// do not exit here as repositoryNames in opts may have the info.
				log.Printf("selected visibility with no repos mentioned for variable %s", row[0])
			}
			repoIdRes := getRepoIds(client, host, opts.OrgName, row[index:])
			if repoIdRes.Err != nil {
				return shared.VariablePayload{}, repoIdRes.Err
			}
			variable.Repositories = repoIdRes.Ids
		}
	}
	return variable, nil
}

func trimSpaces(ss []string) {
	for i := range ss {
		ss[i] = strings.TrimSpace(ss[i])
	}
}

func getBody(opts *SetOptions, client *api.Client, host string, currentVariablePtr []*shared.Variable) (shared.VariablePayload, error) {
	isUpdate := len(currentVariablePtr) > 0

	if opts.Body != "" {
		ss := strings.Split(opts.VariableName+","+string(opts.Body), ",")
		trimSpaces(ss)
		return getVarFromRow(opts, client, host, ss)
	}

	values := make([]string, 0)
	if opts.IO.CanPrompt() {
		var currentVar shared.Variable
		if isUpdate {
			currentVar = *currentVariablePtr[0]
		}
		values = append(values, opts.VariableName)
		data, err := opts.Prompter.Input("Value", currentVar.Value)
		if err != nil {
			return shared.VariablePayload{}, err
		}
		values = append(values, data)

		if opts.OrgName != "" {
			data, err = opts.Prompter.Input("Visibility", string(currentVar.Visibility))
			if err != nil {
				return shared.VariablePayload{}, err
			}
			values = append(values, data)

			if data == shared.Selected {
				data, err = opts.Prompter.Input("Repos(comma separated)", getCurrentSelectedRepos(client.HTTP(), currentVar.SelectedReposURL))
				if err != nil {
					return shared.VariablePayload{}, err
				}
				values = append(values, strings.Split(data, ",")...)
			}
		}

		fmt.Fprintln(opts.IO.Out)
		trimSpaces(values)
		return getVarFromRow(opts, client, host, values)
	}

	body, err := io.ReadAll(opts.IO.In)
	if err != nil {
		return shared.VariablePayload{}, fmt.Errorf("failed to read from standard input: %w", err)
	}
	ss := strings.Split(opts.VariableName+","+string(bytes.TrimRight(body, "\r\n")), ",")
	trimSpaces(ss)
	return getVarFromRow(opts, client, host, ss)
}

// This does similar logic to `api.RepoNetwork`, but without the overfetching.
func mapRepoToID(client *api.Client, host string, repositories []ghrepo.Interface) ([]int64, error) {
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

func getCurrentSelectedRepos(client *http.Client, url string) string {
	var selectedRepositories shared.SelectedRepos
	err := shared.ApiGet(client, url, &selectedRepositories)
	if err != nil {
		return ""
	}
	names := make([]string, 0)
	for _, repo := range selectedRepositories.Repositories {
		names = append(names, repo.Name)
	}
	return strings.Join(names, ",")
}

func mapRepoNamesToIDs(client *api.Client, host, defaultOwner string, repositoryNames []string) ([]int64, error) {
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
	repositoryIDs, err := mapRepoToID(client, host, repos)
	if err != nil {
		return nil, fmt.Errorf("failed to look up IDs for repositories %v: %w", repositoryNames, err)
	}
	return repositoryIDs, nil
}

func getRepoIds(client *api.Client, host, orgName string, repoNames []string) repoNamesResult {
	repoNamesC := make(chan repoNamesResult, 1)
	go func() {
		if len(repoNames) == 0 {
			repoNamesC <- repoNamesResult{}
			return
		}
		repositoryIDs, err := mapRepoNamesToIDs(client, host, orgName, repoNames)
		repoNamesC <- repoNamesResult{
			Ids: repositoryIDs,
			Err: err,
		}
	}()
	return <-repoNamesC
}
