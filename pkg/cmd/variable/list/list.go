package list

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	ghContext "github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/internal/gh"
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
	Config     func() (gh.Config, error)
	BaseRepo   func() (ghrepo.Interface, error)
	Remotes    func() (ghContext.Remotes, error)
	Now        func() time.Time

	Exporter cmdutil.Exporter

	OrgName string
	EnvName string

	HasRepoOverride bool
}

const fieldNumSelectedRepos = "numSelectedRepos"

func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		IO:         f.IOStreams,
		Config:     f.Config,
		HttpClient: f.HttpClient,
		Remotes:    f.Remotes,
		Now:        time.Now,
	}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List variables",
		Long: heredoc.Doc(`
			List variables on one of the following levels:
			- repository (default): available to GitHub Actions runs or Dependabot in a repository
			- environment: available to GitHub Actions runs for a deployment environment in a repository
			- organization: available to GitHub Actions runs or Dependabot within an organization
		`),
		Aliases: []string{"ls"},
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if err := cmdutil.MutuallyExclusive("specify only one of `--org` or `--env`", opts.OrgName != "", opts.EnvName != ""); err != nil {
				return err
			}

			opts.HasRepoOverride = cmd.Flags().Changed("repo")

			if runF != nil {
				return runF(opts)
			}

			return listRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.OrgName, "org", "o", "", "List variables for an organization")
	cmd.Flags().StringVarP(&opts.EnvName, "env", "e", "", "List variables for an environment")
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, shared.VariableJSONFields)

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

		err = cmdutil.ValidateHasOnlyOneRemote(opts.HasRepoOverride, opts.Remotes)
		if err != nil {
			return err
		}
	}

	variableEntity, err := shared.GetVariableEntity(orgName, envName)
	if err != nil {
		return err
	}

	// Since populating the `NumSelectedRepos` field costs further API requests
	// (one per secret), it's important to avoid extra calls when the output will
	// not present the field's value. So, we should only populate this field in
	// these cases:
	//  1. The command is run in the TTY mode without the `--json <fields>` option.
	//  2. The command is run with `--json <fields>` option, and `numSelectedRepos`
	//     is among the selected fields. In this case, TTY mode is irrelevant.
	showSelectedRepoInfo := opts.IO.IsStdoutTTY()
	if opts.Exporter != nil {
		// Note that if there's an exporter set, then we don't mind the TTY mode
		// because we just have to populate the requested fields.
		showSelectedRepoInfo = slices.Contains(opts.Exporter.Fields(), fieldNumSelectedRepos)
	}

	var variables []shared.Variable
	switch variableEntity {
	case shared.Repository:
		variables, err = getRepoVariables(client, baseRepo)
	case shared.Environment:
		variables, err = getEnvVariables(client, baseRepo, envName)
	case shared.Organization:
		var cfg gh.Config
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

	if len(variables) == 0 && opts.Exporter == nil {
		return cmdutil.NewNoResultsError("no variables found")
	}

	if err := opts.IO.StartPager(); err == nil {
		defer opts.IO.StopPager()
	} else {
		fmt.Fprintf(opts.IO.ErrOut, "failed to start pager: %v\n", err)
	}

	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, variables)
	}

	var headers []string
	if variableEntity == shared.Organization {
		headers = []string{"Name", "Value", "Updated", "Visibility"}
	} else {
		headers = []string{"Name", "Value", "Updated"}
	}

	table := tableprinter.New(opts.IO, tableprinter.WithHeader(headers...))
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

func fmtVisibility(s shared.Variable) string {
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

func getRepoVariables(client *http.Client, repo ghrepo.Interface) ([]shared.Variable, error) {
	return getVariables(client, repo.RepoHost(), fmt.Sprintf("repos/%s/actions/variables", ghrepo.FullName(repo)))
}

func getEnvVariables(client *http.Client, repo ghrepo.Interface, envName string) ([]shared.Variable, error) {
	path := fmt.Sprintf("repos/%s/environments/%s/variables", ghrepo.FullName(repo), envName)
	return getVariables(client, repo.RepoHost(), path)
}

func getOrgVariables(client *http.Client, host, orgName string, showSelectedRepoInfo bool) ([]shared.Variable, error) {
	variables, err := getVariables(client, host, fmt.Sprintf("orgs/%s/actions/variables", orgName))
	if err != nil {
		return nil, err
	}
	apiClient := api.NewClientFromHTTP(client)
	if showSelectedRepoInfo {
		err = shared.PopulateMultipleSelectedRepositoryInformation(apiClient, host, variables)
		if err != nil {
			return nil, err
		}
	}
	return variables, nil
}

func getVariables(client *http.Client, host, path string) ([]shared.Variable, error) {
	var results []shared.Variable
	apiClient := api.NewClientFromHTTP(client)
	path = fmt.Sprintf("%s?per_page=100", path)
	for path != "" {
		response := struct {
			Variables []shared.Variable
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
