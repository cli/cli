package set

import (
	"bytes"
	"fmt"
	"github.com/cli/cli/v2/internal/prompter"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	ghContext "github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/variable/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/hashicorp/go-multierror"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

type SetOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	Config     func() (gh.Config, error)
	BaseRepo   func() (ghrepo.Interface, error)
	Remotes    func() (ghContext.Remotes, error)
	Prompter   prompter.Prompter

	VariableName    string
	OrgName         string
	EnvName         string
	Body            string
	Visibility      string
	RepositoryNames []string
	EnvFile         string

	HasRepoOverride bool
	Interactive     bool
}

func NewCmdSet(f *cmdutil.Factory, runF func(*SetOptions) error) *cobra.Command {
	opts := &SetOptions{
		IO:         f.IOStreams,
		Config:     f.Config,
		HttpClient: f.HttpClient,
		Remotes:    f.Remotes,
		Prompter:   f.Prompter,
	}

	cmd := &cobra.Command{
		Use:   "set <variable-name>",
		Short: "Create or update variables",
		Long: heredoc.Doc(`
			Set a value for a variable on one of the following levels:
			- repository (default): available to GitHub Actions runs or Dependabot in a repository
			- environment: available to GitHub Actions runs for a deployment environment in a repository
			- organization: available to GitHub Actions runs or Dependabot within an organization

			Organization variable can optionally be restricted to only be available to
			specific repositories.
		`),
		Example: heredoc.Doc(`
			# Add variable value for the current repository in an interactive prompt
			$ gh variable set MYVARIABLE

			# Read variable value from an environment variable
			$ gh variable set MYVARIABLE --body "$ENV_VALUE"

			# Set secret for a specific remote repository
			$ gh variable set MYVARIABLE --repo origin/repo --body "$ENV_VALUE"

			# Read variable value from a file
			$ gh variable set MYVARIABLE < myfile.txt

			# Set variable for a deployment environment in the current repository
			$ gh variable set MYVARIABLE --env myenvironment

			# Set organization-level variable visible to both public and private repositories
			$ gh variable set MYVARIABLE --org myOrg --visibility all

			# Set organization-level variable visible to specific repositories
			$ gh variable set MYVARIABLE --org myOrg --repos repo1,repo2,repo3

			# Set multiple variables imported from the ".env" file
			$ gh variable set -f .env
		`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			flagCount := cmdutil.CountSetFlags(cmd.Flags())

			if err := cmdutil.MutuallyExclusive("specify only one of `--org` or `--env`", opts.OrgName != "", opts.EnvName != ""); err != nil {
				return err
			}

			if err := cmdutil.MutuallyExclusive("specify only one of `--body` or `--env-file`", opts.Body != "", opts.EnvFile != ""); err != nil {
				return err
			}

			if len(args) == 0 {
				if opts.EnvFile == "" {
					return cmdutil.FlagErrorf("must pass name argument")
				}
			} else {
				opts.VariableName = args[0]

				if flagCount == 0 {
					opts.Interactive = true
				}
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

			opts.HasRepoOverride = cmd.Flags().Changed("repo")

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
	cmd.Flags().StringVarP(&opts.EnvFile, "env-file", "f", "", "Load variable names and values from a dotenv-formatted `file`")

	return cmd
}

func setRun(opts *SetOptions) error {
	variables, err := getVariablesFromOptions(opts)
	if err != nil {
		return err
	}

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

		err = cmdutil.ValidateHasOnlyOneRemote(opts.HasRepoOverride, opts.Remotes)
		if err != nil {
			if opts.Interactive {
				selectedRepo, errSelectedRepo := cmdutil.PromptForRepo(baseRepo, opts.Remotes, opts.Prompter)

				if errSelectedRepo != nil {
					return errSelectedRepo
				}

				baseRepo = selectedRepo
			} else {
				return err
			}
		}

		host = baseRepo.RepoHost()
	} else {
		cfg, err := opts.Config()
		if err != nil {
			return err
		}
		host, _ = cfg.Authentication().DefaultHost()
	}

	entity, err := shared.GetVariableEntity(orgName, envName)
	if err != nil {
		return err
	}

	opts.IO.StartProgressIndicator()
	repositoryIDs, err := getRepoIds(client, host, opts.OrgName, opts.RepositoryNames)
	opts.IO.StopProgressIndicator()
	if err != nil {
		return err
	}

	setc := make(chan setResult)
	for key, value := range variables {
		k := key
		v := value
		go func() {
			setOpts := setOptions{
				Entity:        entity,
				Environment:   envName,
				Key:           k,
				Organization:  orgName,
				Repository:    baseRepo,
				RepositoryIDs: repositoryIDs,
				Value:         v,
				Visibility:    opts.Visibility,
			}
			setc <- setVariable(client, host, setOpts)
		}()
	}

	err = nil
	cs := opts.IO.ColorScheme()
	for i := 0; i < len(variables); i++ {
		result := <-setc
		if result.Err != nil {
			err = multierror.Append(err, result.Err)
			continue
		}
		if !opts.IO.IsStdoutTTY() {
			continue
		}
		target := orgName
		if orgName == "" {
			target = ghrepo.FullName(baseRepo)
		}
		if envName != "" {
			target += " environment " + envName
		}
		fmt.Fprintf(opts.IO.Out, "%s %s variable %s for %s\n", cs.SuccessIcon(), result.Operation, result.Key, target)
	}

	return err
}

func getVariablesFromOptions(opts *SetOptions) (map[string]string, error) {
	variables := make(map[string]string)

	if opts.EnvFile != "" {
		var r io.Reader
		if opts.EnvFile == "-" {
			defer opts.IO.In.Close()
			r = opts.IO.In
		} else {
			f, err := os.Open(opts.EnvFile)
			if err != nil {
				return nil, fmt.Errorf("failed to open env file: %w", err)
			}
			defer f.Close()
			r = f
		}
		envs, err := godotenv.Parse(r)
		if err != nil {
			return nil, fmt.Errorf("error parsing env file: %w", err)
		}
		if len(envs) == 0 {
			return nil, fmt.Errorf("no variables found in file")
		}
		for key, value := range envs {
			variables[key] = value
		}
		return variables, nil
	}

	body, err := getBody(opts)
	if err != nil {
		return nil, fmt.Errorf("did not understand variable body: %w", err)
	}
	variables[opts.VariableName] = body

	return variables, nil
}

func getBody(opts *SetOptions) (string, error) {
	if opts.Body != "" {
		return opts.Body, nil
	}

	if opts.IO.CanPrompt() {
		bodyInput, err := opts.Prompter.Input("Paste your variable", "")
		if err != nil {
			return "", err
		}
		fmt.Fprintln(opts.IO.Out)
		return bodyInput, nil
	}

	body, err := io.ReadAll(opts.IO.In)
	if err != nil {
		return "", fmt.Errorf("failed to read from standard input: %w", err)
	}

	return string(bytes.TrimRight(body, "\r\n")), nil
}

func getRepoIds(client *api.Client, host, owner string, repositoryNames []string) ([]int64, error) {
	if len(repositoryNames) == 0 {
		return nil, nil
	}
	repos := make([]ghrepo.Interface, 0, len(repositoryNames))
	for _, repositoryName := range repositoryNames {
		if repositoryName == "" {
			continue
		}
		var repo ghrepo.Interface
		if strings.Contains(repositoryName, "/") || owner == "" {
			var err error
			repo, err = ghrepo.FromFullNameWithHost(repositoryName, host)
			if err != nil {
				return nil, fmt.Errorf("invalid repository name")
			}
		} else {
			repo = ghrepo.NewWithHost(owner, repositoryName, host)
		}
		repos = append(repos, repo)
	}
	if len(repos) == 0 {
		return nil, fmt.Errorf("resetting repositories selected to zero is not supported")
	}
	repositoryIDs, err := api.GetRepoIDs(client, host, repos)
	if err != nil {
		return nil, fmt.Errorf("failed to look up IDs for repositories %v: %w", repositoryNames, err)
	}
	return repositoryIDs, nil
}
