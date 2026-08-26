package list

import (
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/internal/safeurl"
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
	Prompter   prompter.Prompter

	Now      func() time.Time
	Exporter cmdutil.Exporter

	OrgName     string
	EnvName     string
	UserSecrets bool
	Application string
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
		Now:        time.Now,
		Prompter:   f.Prompter,
	}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List secrets",
		Long: heredoc.Doc(`
			List secrets on one of the following levels:
			- repository (default): available to GitHub Actions runs, Agents sessions, or Dependabot in a repository
			- environment: available to GitHub Actions runs for a deployment environment in a repository
			- organization: available to GitHub Actions runs, Agents sessions, Dependabot, or Codespaces within an organization
			- user: available to Codespaces for your user
		`),
		Aliases: []string{"ls"},
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// If the user specified a repo directly, then we're using the OverrideBaseRepoFunc set by EnableRepoOverride
			// So there's no reason to use the specialised BaseRepoFunc that requires remote disambiguation.
			opts.BaseRepo = f.BaseRepo
			isRepoUserProvided := cmd.Flags().Changed("repo") || os.Getenv("GH_REPO") != ""
			if !isRepoUserProvided {
				// If they haven't specified a repo directly, then we will wrap the BaseRepoFunc in one that errors if
				// there might be multiple valid remotes.
				opts.BaseRepo = shared.RequireNoAmbiguityBaseRepoFunc(opts.BaseRepo, f.Remotes)
				// But if we are able to prompt, then we will wrap that up in a BaseRepoFunc that can prompt the user to
				// resolve the ambiguity.
				if opts.IO.CanPrompt() {
					opts.BaseRepo = shared.PromptWhenAmbiguousBaseRepoFunc(opts.BaseRepo, f.IOStreams, f.Prompter)
				}
			}

			if err := cmdutil.MutuallyExclusive("specify only one of `--org`, `--env`, or `--user`", opts.OrgName != "", opts.EnvName != "", opts.UserSecrets); err != nil {
				return err
			}

			if runF != nil {
				return runF(opts)
			}

			return listRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.OrgName, "org", "o", "", "List secrets for an organization")
	cmd.Flags().StringVarP(&opts.EnvName, "env", "e", "", "List secrets for an environment")
	cmd.Flags().BoolVarP(&opts.UserSecrets, "user", "u", false, "List a secret for your user")
	cmdutil.StringEnumFlag(cmd, &opts.Application, "app", "a", "", []string{shared.Actions, shared.Agents, shared.Codespaces, shared.Dependabot}, "List secrets for a specific application")
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

func (s *Secret) ExportData(fields []string) map[string]any {
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
	u, err := safeurl.JoinPath("orgs", orgName, string(app), "secrets")
	if err != nil {
		return nil, err
	}
	secrets, err := getSecrets(client, host, u)
	if err != nil {
		return nil, err
	}

	if showSelectedRepoInfo {
		for i := range secrets {
			if secrets[i].SelectedReposURL == "" {
				continue
			}
			count, err := selectedRepositoryCount(client, host, safeurl.NewImmutableSafeURL(secrets[i].SelectedReposURL))
			if err != nil {
				return nil, fmt.Errorf("failed determining selected repositories for %s: %w", secrets[i].Name, err)
			}
			secrets[i].NumSelectedRepos = count
		}
	}
	return secrets, nil
}

func getUserSecrets(client *http.Client, host string, showSelectedRepoInfo bool) ([]Secret, error) {
	u, err := safeurl.JoinPath("user", "codespaces", "secrets")
	if err != nil {
		return nil, err
	}
	secrets, err := getSecrets(client, host, u)
	if err != nil {
		return nil, err
	}

	if showSelectedRepoInfo {
		for i := range secrets {
			if secrets[i].SelectedReposURL == "" {
				continue
			}
			count, err := selectedRepositoryCount(client, host, safeurl.NewImmutableSafeURL(secrets[i].SelectedReposURL))
			if err != nil {
				return nil, fmt.Errorf("failed determining selected repositories for %s: %w", secrets[i].Name, err)
			}
			secrets[i].NumSelectedRepos = count
		}
	}

	return secrets, nil
}

func getEnvSecrets(client *http.Client, repo ghrepo.Interface, envName string) ([]Secret, error) {
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "environments", envName, "secrets")
	if err != nil {
		return nil, err
	}
	return getSecrets(client, repo.RepoHost(), path)
}

func getRepoSecrets(client *http.Client, repo ghrepo.Interface, app shared.App) ([]Secret, error) {
	u, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), string(app), "secrets")
	if err != nil {
		return nil, err
	}
	return getSecrets(client, repo.RepoHost(), u)
}

func getSecrets(client *http.Client, host string, u *safeurl.MutableSafeURL) ([]Secret, error) {
	var results []Secret
	apiClient := api.NewClientFromHTTP(client)
	u.SetQuery("per_page", "100")
	var pageURL safeurl.SafeURL = u
	for pageURL.String() != "" {
		response := struct {
			Secrets []Secret
		}{}
		next, err := apiClient.RESTWithNext(host, "GET", pageURL.String(), nil, &response)
		if err != nil {
			return nil, err
		}
		pageURL = safeurl.NewImmutableSafeURL(next)
		results = append(results, response.Secrets...)
	}
	return results, nil
}

func selectedRepositoryCount(client *http.Client, host string, selectedReposURL safeurl.SafeURL) (int, error) {
	apiClient := api.NewClientFromHTTP(client)
	response := struct {
		TotalCount int `json:"total_count"`
	}{}
	if err := apiClient.REST(host, "GET", selectedReposURL.String(), nil, &response); err != nil {
		return 0, err
	}
	return response.TotalCount, nil
}
