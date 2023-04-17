package list

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/pkg/cmd/variable/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type ListOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	Config     func() (config.Config, error)
	BaseRepo   func() (ghrepo.Interface, error)
	Now        func() time.Time

	OrgName string
	EnvName string
}

func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		IO:         f.IOStreams,
		Config:     f.Config,
		HttpClient: f.HttpClient,
		Now:        time.Now,
	}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List variables",
		Long: heredoc.Doc(`
			List variables on one of the following levels:
			- repository (default): available to Actions runs or Dependabot in a repository
			- environment: available to Actions runs for a deployment environment in a repository
			- organization: available to Actions runs or Dependabot within an organization
		`),
		Aliases: []string{"ls"},
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if err := cmdutil.MutuallyExclusive("specify only one of `--org` or `--env`", opts.OrgName != "", opts.EnvName != ""); err != nil {
				return err
			}

			if runF != nil {
				return runF(opts)
			}

			return listRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.OrgName, "org", "o", "", "List variables for an organization")
	cmd.Flags().StringVarP(&opts.EnvName, "env", "e", "", "List variables for an environment")

	return cmd
}

func listRun(opts *ListOptions) error {
	client, err := opts.HttpClient()
	if err != nil {
		return fmt.Errorf("could not create http client: %w", err)
	}

	orgName := opts.OrgName
	envName := opts.EnvName

	var baseRepo ghrepo.Interface
	if orgName == "" {
		baseRepo, err = opts.BaseRepo()
		if err != nil {
			return err
		}
	}

	variableEntity, err := shared.GetVariableEntity(orgName, envName)
	if err != nil {
		return err
	}

	var variables []Variable
	showSelectedRepoInfo := opts.IO.IsStdoutTTY()

	switch variableEntity {
	case shared.Repository:
		variables, err = getRepoVariables(client, baseRepo)
	case shared.Environment:
		variables, err = getEnvVariables(client, baseRepo, envName)
	case shared.Organization:
		var cfg config.Config
		var host string
		cfg, err = opts.Config()
		if err != nil {
			return err
		}
		host, _ = cfg.Authentication().DefaultHost()
		variables, err = getOrgVariables(client, host, orgName, showSelectedRepoInfo)
	}

	if err != nil {
		return fmt.Errorf("failed to get variables: %w", err)
	}

	if len(variables) == 0 {
		return cmdutil.NewNoResultsError("no variables found")
	}

	if err := opts.IO.StartPager(); err == nil {
		defer opts.IO.StopPager()
	} else {
		fmt.Fprintf(opts.IO.ErrOut, "failed to start pager: %v\n", err)
	}

	table := tableprinter.New(opts.IO)
	if variableEntity == shared.Organization {
		table.HeaderRow("Name", "Value", "Updated", "Visibility")
	} else {
		table.HeaderRow("Name", "Value", "Updated")
	}
	for _, variable := range variables {
		table.AddField(variable.Name)
		table.AddField(variable.Value)
		table.AddTimeField(opts.Now(), variable.UpdatedAt, nil)
		if variable.Visibility != "" {
			if showSelectedRepoInfo {
				table.AddField(fmtVisibility(variable))
			} else {
				table.AddField(strings.ToUpper(string(variable.Visibility)))
			}
		}
		table.EndRow()
	}

	err = table.Render()
	if err != nil {
		return err
	}

	return nil
}

type Variable struct {
	Name             string
	Value            string
	UpdatedAt        time.Time `json:"updated_at"`
	Visibility       shared.Visibility
	SelectedReposURL string `json:"selected_repositories_url"`
	NumSelectedRepos int
}

func fmtVisibility(s Variable) string {
	switch s.Visibility {
	case shared.All:
		return "Visible to all repositories"
	case shared.Private:
		return "Visible to private repositories"
	case shared.Selected:
		if s.NumSelectedRepos == 1 {
			return "Visible to 1 selected repository"
		} else {
			return fmt.Sprintf("Visible to %d selected repositories", s.NumSelectedRepos)
		}
	}
	return ""
}

func getRepoVariables(client *http.Client, repo ghrepo.Interface) ([]Variable, error) {
	return getVariables(client, repo.RepoHost(), fmt.Sprintf("repos/%s/actions/variables", ghrepo.FullName(repo)))
}

func getEnvVariables(client *http.Client, repo ghrepo.Interface, envName string) ([]Variable, error) {
	path := fmt.Sprintf("repos/%s/environments/%s/variables", ghrepo.FullName(repo), envName)
	return getVariables(client, repo.RepoHost(), path)
}

func getOrgVariables(client *http.Client, host, orgName string, showSelectedRepoInfo bool) ([]Variable, error) {
	variables, err := getVariables(client, host, fmt.Sprintf("orgs/%s/actions/variables", orgName))
	if err != nil {
		return nil, err
	}
	if showSelectedRepoInfo {
		err = populateSelectedRepositoryInformation(client, host, variables)
		if err != nil {
			return nil, err
		}
	}
	return variables, nil
}

func getVariables(client *http.Client, host, path string) ([]Variable, error) {
	var results []Variable
	apiClient := api.NewClientFromHTTP(client)
	path = fmt.Sprintf("%s?per_page=100", path)
	for path != "" {
		response := struct {
			Variables []Variable
		}{}
		var err error
		path, err = apiClient.RESTWithNext(host, "GET", path, nil, &response)
		if err != nil {
			return nil, err
		}
		results = append(results, response.Variables...)
	}
	return results, nil
}

func populateSelectedRepositoryInformation(client *http.Client, host string, variables []Variable) error {
	apiClient := api.NewClientFromHTTP(client)
	for i, variable := range variables {
		if variable.SelectedReposURL == "" {
			continue
		}
		response := struct {
			TotalCount int `json:"total_count"`
		}{}
		if err := apiClient.REST(host, "GET", variable.SelectedReposURL, nil, &response); err != nil {
			return fmt.Errorf("failed determining selected repositories for %s: %w", variable.Name, err)
		}
		variables[i].NumSelectedRepos = response.TotalCount
	}
	return nil
}
