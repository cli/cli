package list

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/secret/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/utils"
	"github.com/spf13/cobra"
)

type ListOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	Config     func() (config.Config, error)
	BaseRepo   func() (ghrepo.Interface, error)

	OrgName     string
	EnvName     string
	UserSecrets bool
	Application string
}

func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		IO:         f.IOStreams,
		Config:     f.Config,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List secrets",
		Long: heredoc.Doc(`
			List secrets on one of the following levels:
			- repository (default): available to Actions runs or Dependabot in a repository
			- environment: available to Actions runs for a deployment environment in a repository
			- organization: available to Actions runs or Dependabot within an organization
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
			return fmt.Errorf("could not determine base repo: %w", err)
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

	var secrets []*Secret
	showSelectedRepoInfo := opts.IO.IsStdoutTTY()

	switch secretEntity {
	case shared.Repository:
		secrets, err = getRepoSecrets(client, baseRepo, secretApp)
	case shared.Environment:
		secrets, err = getEnvSecrets(client, baseRepo, envName)
	case shared.Organization, shared.User:
		var cfg config.Config
		var host string

		cfg, err = opts.Config()
		if err != nil {
			return err
		}

		host, err = cfg.DefaultHost()
		if err != nil {
			return err
		}

		if secretEntity == shared.User {
			secrets, err = getUserSecrets(client, host, showSelectedRepoInfo)
		} else {
			secrets, err = getOrgSecrets(client, host, orgName, showSelectedRepoInfo, secretApp)
		}
	}

	if err != nil {
		return fmt.Errorf("failed to get secrets: %w", err)
	}

	if err := opts.IO.StartPager(); err == nil {
		defer opts.IO.StopPager()
	} else {
		fmt.Fprintf(opts.IO.ErrOut, "failed to start pager: %v\n", err)
	}

	tp := utils.NewTablePrinter(opts.IO)
	for _, secret := range secrets {
		tp.AddField(secret.Name, nil, nil)
		updatedAt := secret.UpdatedAt.Format("2006-01-02")
		if opts.IO.IsStdoutTTY() {
			updatedAt = fmt.Sprintf("Updated %s", updatedAt)
		}
		tp.AddField(updatedAt, nil, nil)
		if secret.Visibility != "" {
			if showSelectedRepoInfo {
				tp.AddField(fmtVisibility(*secret), nil, nil)
			} else {
				tp.AddField(strings.ToUpper(string(secret.Visibility)), nil, nil)
			}
		}
		tp.EndRow()
	}

	err = tp.Render()
	if err != nil {
		return err
	}

	return nil
}

type Secret struct {
	Name             string
	UpdatedAt        time.Time `json:"updated_at"`
	Visibility       shared.Visibility
	SelectedReposURL string `json:"selected_repositories_url"`
	NumSelectedRepos int
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

func getOrgSecrets(client httpClient, host, orgName string, showSelectedRepoInfo bool, app shared.App) ([]*Secret, error) {
	secrets, err := getSecrets(client, host, fmt.Sprintf("orgs/%s/%s/secrets", orgName, app))
	if err != nil {
		return nil, err
	}

	if showSelectedRepoInfo {
		err = getSelectedRepositoryInformation(client, secrets)
		if err != nil {
			return nil, err
		}
	}
	return secrets, nil
}

func getUserSecrets(client httpClient, host string, showSelectedRepoInfo bool) ([]*Secret, error) {
	secrets, err := getSecrets(client, host, "user/codespaces/secrets")
	if err != nil {
		return nil, err
	}

	if showSelectedRepoInfo {
		err = getSelectedRepositoryInformation(client, secrets)
		if err != nil {
			return nil, err
		}
	}

	return secrets, nil
}

func getEnvSecrets(client httpClient, repo ghrepo.Interface, envName string) ([]*Secret, error) {
	path := fmt.Sprintf("repos/%s/environments/%s/secrets", ghrepo.FullName(repo), envName)
	return getSecrets(client, repo.RepoHost(), path)
}

func getRepoSecrets(client httpClient, repo ghrepo.Interface, app shared.App) ([]*Secret, error) {
	return getSecrets(client, repo.RepoHost(), fmt.Sprintf("repos/%s/%s/secrets",
		ghrepo.FullName(repo), app))
}

type secretsPayload struct {
	Secrets []*Secret
}

type httpClient interface {
	Do(*http.Request) (*http.Response, error)
}

func getSecrets(client httpClient, host, path string) ([]*Secret, error) {
	var results []*Secret
	url := fmt.Sprintf("%s%s?per_page=100", ghinstance.RESTPrefix(host), path)

	for {
		var payload secretsPayload
		nextURL, err := apiGet(client, url, &payload)
		if err != nil {
			return nil, err
		}
		results = append(results, payload.Secrets...)

		if nextURL == "" {
			break
		}
		url = nextURL
	}

	return results, nil
}

func apiGet(client httpClient, url string, data interface{}) (string, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode > 299 {
		return "", api.HandleHTTPError(resp)
	}

	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(data); err != nil {
		return "", err
	}

	return findNextPage(resp.Header.Get("Link")), nil
}

var linkRE = regexp.MustCompile(`<([^>]+)>;\s*rel="([^"]+)"`)

func findNextPage(link string) string {
	for _, m := range linkRE.FindAllStringSubmatch(link, -1) {
		if len(m) > 2 && m[2] == "next" {
			return m[1]
		}
	}
	return ""
}

func getSelectedRepositoryInformation(client httpClient, secrets []*Secret) error {
	type responseData struct {
		TotalCount int `json:"total_count"`
	}

	for _, secret := range secrets {
		if secret.SelectedReposURL == "" {
			continue
		}
		var result responseData
		if _, err := apiGet(client, secret.SelectedReposURL, &result); err != nil {
			return fmt.Errorf("failed determining selected repositories for %s: %w", secret.Name, err)
		}
		secret.NumSelectedRepos = result.TotalCount
	}

	return nil
}
