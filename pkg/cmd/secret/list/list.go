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
	"github.com/cli/cli/v2/pkg/cmd/secret/shared"
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
	Exporter   cmdutil.Exporter

	OrgName     string
	EnvName     string
	UserSecrets bool
	Application string

	HasRepoOverride bool
}

var secretFields = []string{
	"selectedReposURL",
	"name",
	"visibility",
	"updatedAt",
	"numSelectedRepos",
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
		Short: "List secrets",
		Long: heredoc.Doc(`
			List secrets on one of the following levels:
			- repository (default): available to GitHub Actions runs or Dependabot in a repository
			- environment: available to GitHub Actions runs for a deployment environment in a repository
			- organization: available to GitHub Actions runs, Dependabot, or Codespaces within an organization
			- user: available to Codespaces for your user
		`),
		Aliases: []string{"ls"},
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if err := cmdutil.MutuallyExclusive("specify only one of `--org`, `--env`, or `--user`", opts.OrgName != "", opts.EnvName != "", opts.UserSecrets); err != nil {
				return err
			}

			opts.HasRepoOverride = cmd.Flags().Changed("repo")

			if runF != nil {
				return runF(opts)
			}

			return listRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.OrgName, "org", "o", "", "List secrets for an organization")
	cmd.Flags().StringVarP(&opts.EnvName, "env", "e", "", "List secrets for an environment")
	cmd.Flags().BoolVarP(&opts.UserSecrets, "user", "u", false, "List a secret for your user")
	cmdutil.StringEnumFlag(cmd, &opts.Application, "app", "a", "", []string{shared.Actions, shared.Codespaces, shared.Dependabot}, "List secrets for a specific application")
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, secretFields)
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
	if orgName == "" && !opts.UserSecrets {
		baseRepo, err = opts.BaseRepo()
		if err != nil {
			return err
		}

		err = cmdutil.ValidateHasOnlyOneRemote(opts.HasRepoOverride, opts.Remotes)
		if err != nil {
			return err
		}
	}

	secretEntity, err := shared.GetSecretEntity(orgName, envName, opts.UserSecrets)
	if err != nil {
		return err
	}

	secretApp, err := shared.GetSecretApp(opts.Application, secretEntity)
	if err != nil {
		return err
	}

	if !shared.IsSupportedSecretEntity(secretApp, secretEntity) {
		return fmt.Errorf("%s secrets are not supported for %s", secretEntity, secretApp)
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

	var secrets []Secret
	switch secretEntity {
	case shared.Repository:
		secrets, err = getRepoSecrets(client, baseRepo, secretApp)
	case shared.Environment:
		secrets, err = getEnvSecrets(client, baseRepo, envName)
	case shared.Organization, shared.User:
		var cfg gh.Config
		var host string

		cfg, err = opts.Config()
		if err != nil {
			return err
		}

		host, _ = cfg.Authentication().DefaultHost()

		if secretEntity == shared.User {
			secrets, err = getUserSecrets(client, host, showSelectedRepoInfo)
		} else {
			secrets, err = getOrgSecrets(client, host, orgName, showSelectedRepoInfo, secretApp)
		}
	}

	if err != nil {
		return fmt.Errorf("failed to get secrets: %w", err)
	}

	if len(secrets) == 0 && opts.Exporter == nil {
		return cmdutil.NewNoResultsError("no secrets found")
	}

	if err := opts.IO.StartPager(); err == nil {
		defer opts.IO.StopPager()
	} else {
		fmt.Fprintf(opts.IO.ErrOut, "failed to start pager: %v\n", err)
	}

	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, secrets)
	}

	var headers []string
	if secretEntity == shared.Organization || secretEntity == shared.User {
		headers = []string{"Name", "Updated", "Visibility"}
	} else {
		headers = []string{"Name", "Updated"}
	}

	table := tableprinter.New(opts.IO, tableprinter.WithHeader(headers...))
	for _, secret := range secrets {
		table.AddField(secret.Name)
		table.AddTimeField(opts.Now(), secret.UpdatedAt, nil)
		if secret.Visibility != "" {
			if showSelectedRepoInfo {
				table.AddField(fmtVisibility(secret))
			} else {
				table.AddField(strings.ToUpper(string(secret.Visibility)))
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

type Secret struct {
	Name             string            `json:"name"`
	UpdatedAt        time.Time         `json:"updated_at"`
	Visibility       shared.Visibility `json:"visibility"`
	SelectedReposURL string            `json:"selected_repositories_url"`
	NumSelectedRepos int               `json:"num_selected_repos"`
}

func (s *Secret) ExportData(fields []string) map[string]interface{} {
	return cmdutil.StructExportData(s, fields)
}

func fmtVisibility(s Secret) string {
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

func getOrgSecrets(client *http.Client, host, orgName string, showSelectedRepoInfo bool, app shared.App) ([]Secret, error) {
	secrets, err := getSecrets(client, host, fmt.Sprintf("orgs/%s/%s/secrets", orgName, app))
	if err != nil {
		return nil, err
	}

	if showSelectedRepoInfo {
		err = populateSelectedRepositoryInformation(client, host, secrets)
		if err != nil {
			return nil, err
		}
	}
	return secrets, nil
}

func getUserSecrets(client *http.Client, host string, showSelectedRepoInfo bool) ([]Secret, error) {
	secrets, err := getSecrets(client, host, "user/codespaces/secrets")
	if err != nil {
		return nil, err
	}

	if showSelectedRepoInfo {
		err = populateSelectedRepositoryInformation(client, host, secrets)
		if err != nil {
			return nil, err
		}
	}

	return secrets, nil
}

func getEnvSecrets(client *http.Client, repo ghrepo.Interface, envName string) ([]Secret, error) {
	path := fmt.Sprintf("repos/%s/environments/%s/secrets", ghrepo.FullName(repo), envName)
	return getSecrets(client, repo.RepoHost(), path)
}

func getRepoSecrets(client *http.Client, repo ghrepo.Interface, app shared.App) ([]Secret, error) {
	return getSecrets(client, repo.RepoHost(), fmt.Sprintf("repos/%s/%s/secrets", ghrepo.FullName(repo), app))
}

func getSecrets(client *http.Client, host, path string) ([]Secret, error) {
	var results []Secret
	apiClient := api.NewClientFromHTTP(client)
	path = fmt.Sprintf("%s?per_page=100", path)
	for path != "" {
		response := struct {
			Secrets []Secret
		}{}
		var err error
		path, err = apiClient.RESTWithNext(host, "GET", path, nil, &response)
		if err != nil {
			return nil, err
		}
		results = append(results, response.Secrets...)
	}
	return results, nil
}

func populateSelectedRepositoryInformation(client *http.Client, host string, secrets []Secret) error {
	apiClient := api.NewClientFromHTTP(client)
	for i, secret := range secrets {
		if secret.SelectedReposURL == "" {
			continue
		}
		response := struct {
			TotalCount int `json:"total_count"`
		}{}
		if err := apiClient.REST(host, "GET", secret.SelectedReposURL, nil, &response); err != nil {
			return fmt.Errorf("failed determining selected repositories for %s: %w", secret.Name, err)
		}
		secrets[i].NumSelectedRepos = response.TotalCount
	}
	return nil
}
