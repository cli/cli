package create

import (
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/variable/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/hashicorp/go-multierror"
	"github.com/spf13/cobra"
)

func NewCmdCreate(f *cmdutil.Factory, runF func(*shared.PostPatchOptions) error) *cobra.Command {
	opts := &shared.PostPatchOptions{
		IO:         f.IOStreams,
		Config:     f.Config,
		HttpClient: f.HttpClient,
		Prompter:   f.Prompter,
	}

	cmd := &cobra.Command{
		Use:   "create <variable-name>",
		Short: "Create variables",
		Long: heredoc.Doc(`
			Create a value for a variable on one of the following levels:
			- repository (default): available to Actions runs in a repository
			- environment: available to Actions runs for a deployment environment in a repository
			- organization: available to Actions runs within an organization

			Organization can optionally be restricted to only be available to
			specific repositories.

			Variable values are locally value before being sent to GitHub.
		`),
		Example: heredoc.Doc(`
			# Add variable value for the current repository in an interactive prompt
			$ gh variable create MYVARIABLE

			# Read variable value from an environment variable
			$ gh variable create MYVARIABLE --body "$ENV_VALUE"

			# Read variable value from a file
			$ gh variable create MYVARIABLE < myfile.txt

			# Create variable for a deployment environment in the current repository
			$ gh variable create MYVARIABLE --env myenvironment

			# Create organization-level variable visible to both public and private repositories
			$ gh variable create MYVARIABLE --org myOrg --visibility all

			# Create organization-level variable visible to specific repositories
			$ gh variable create MYVARIABLE --org myOrg --repos repo1,repo2,repo3

			# Create multiple variables from the csv in the prescribed format
			$ gh variable create -f file.csv

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

			return createRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.OrgName, "org", "o", "", "Create `organization` variable")
	cmd.Flags().StringVarP(&opts.EnvName, "env", "e", "", "Create deployment `environment` variable")
	cmdutil.StringEnumFlag(cmd, &opts.Visibility, "visibility", "v", shared.Private, []string{shared.All, shared.Private, shared.Selected}, "Create visibility for an organization variable")
	cmd.Flags().StringSliceVarP(&opts.RepositoryNames, "repos", "r", []string{}, "List of `repositories` that can access an organization")
	cmd.Flags().StringVarP(&opts.Body, "body", "b", "", "The value for the variable (reads from standard input if not specified)")
	cmd.Flags().StringVarP(&opts.CsvFile, "csv-file", "f", "", "Load variable names and values from a csv-formatted `file`")
	cmdutil.StringEnumFlag(cmd, &opts.Application, "app", "a", shared.Actions, []string{shared.Actions}, "Create the application for a variable")

	return cmd
}

func createRun(opts *shared.PostPatchOptions) error {

	c, err := opts.HttpClient()
	if err != nil {
		return fmt.Errorf("could not create http client: %w", err)
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
	variables, err := shared.GetVariablesFromOptions(opts, client, host, false)
	if err != nil {
		return err
	}

	variableEntity, err := shared.GetVariableEntity(orgName, envName)
	if err != nil {
		return err
	}

	variableApp, err := shared.GetVariableApp(opts.Application, variableEntity)
	if err != nil || variableApp == shared.Unknown {
		return err
	}

	if !shared.IsSupportedVariableEntity(variableApp, variableEntity) {
		return fmt.Errorf("%s variables are not supported for %s", variableEntity, variableApp)
	}

	repoIdRes := shared.GetRepoIds(client, host, opts.OrgName, opts.RepositoryNames)
	if repoIdRes.Err != nil {
		return repoIdRes.Err
	}

	createc := make(chan createResult)
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
			createc <- createVariable(opts, host, client, baseRepo, variableKey, variable.Value, visibility, repoIds, variableApp, variableEntity)
		}(variableKey, variable)
	}

	err = nil
	cs := opts.IO.ColorScheme()
	for i := 0; i < len(variables); i++ {
		result := <-createc
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
		fmt.Fprintf(opts.IO.Out, "%s Create %s variable %s for %s\n", cs.SuccessIcon(), variableApp.Title(), result.key, target)
	}
	return err
}

type createResult struct {
	key   string
	value string
	err   error
}

func createVariable(opts *shared.PostPatchOptions, host string, client *api.Client, baseRepo ghrepo.Interface, variableKey, variableValue, visibility string, repositoryIDs []int64, app shared.App, entity shared.VariableEntity) (res createResult) {
	orgName := opts.OrgName
	envName := opts.EnvName
	res.key = variableKey
	var err error
	switch entity {
	case shared.Organization:
		err = postOrgVariable(client, host, orgName, visibility, variableKey, variableValue, repositoryIDs, app)
	case shared.Environment:
		err = postEnvVariable(client, baseRepo, envName, variableKey, variableValue)
	default:
		err = postRepoVariable(client, baseRepo, variableKey, variableValue, app)
	}
	if err != nil {
		res.err = fmt.Errorf("failed to create variable %q: %w", variableKey, err)
		return
	}
	return
}
