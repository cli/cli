package update

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

func NewCmdUpdate(f *cmdutil.Factory, runF func(*shared.PostPatchOptions) error) *cobra.Command {
	opts := &shared.PostPatchOptions{
		IO:         f.IOStreams,
		Config:     f.Config,
		HttpClient: f.HttpClient,
		Prompter:   f.Prompter,
	}

	cmd := &cobra.Command{
		Use:   "update <variable-name>",
		Short: "Update variables",
		Long: heredoc.Doc(`
			Update a value for a variable on one of the following levels:
			- repository (default): available to Actions runs in a repository
			- environment: available to Actions runs for a deployment environment in a repository
			- organization: available to Actions runs within an organization

			Organization variables can optionally be restricted to only be available to
			specific repositories.
		`),
		Example: heredoc.Doc(`
			# Add variable value for the current repository in an interactive prompt
			$ gh variable update MYVARIABLE

			# Read variable value from an environment variable
			$ gh variable update MYVARIABLE --body "$ENV_VALUE"

			# Read variable value from a file
			$ gh variable update MYVARIABLE < myfile.txt

			# Update variable for a deployment environment in the current repository
			$ gh variable update MYVARIABLE --env myenvironment

			# Update organization-level variable visible to both public and private repositories
			$ gh variable update MYVARIABLE --org myOrg --visibility all

			# Update organization-level variable visible to specific repositories
			$ gh variable update MYVARIABLE --org myOrg --repos repo1,repo2,repo3

			# Update multiple variables from the csv in the prescribed format
			$ gh variable update -f file.csv

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

			return updateRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.OrgName, "org", "o", "", "Update `organization` variable")
	cmd.Flags().StringVarP(&opts.EnvName, "env", "e", "", "Update deployment `environment` variable")
	cmdutil.StringEnumFlag(cmd, &opts.Visibility, "visibility", "v", shared.Private, []string{shared.All, shared.Private, shared.Selected}, "Update visibility for an organization variable")
	cmd.Flags().StringSliceVarP(&opts.RepositoryNames, "repos", "r", []string{}, "List of `repositories` that can access an organization variable")
	cmd.Flags().StringVarP(&opts.Body, "body", "b", "", "The value for the variable (reads from standard input if not specified)")
	cmd.Flags().StringVarP(&opts.CsvFile, "csv-file", "f", "", "Update names and values from a csv-formatted `file`")

	return cmd
}

func updateRun(opts *shared.PostPatchOptions) error {
	c, err := opts.HttpClient()
	if err != nil {
		return fmt.Errorf("could not update http client: %w", err)
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

	variables, err := shared.GetVariablesFromOptions(opts, client, host, true)
	if err != nil {
		return err
	}

	variableEntity, err := shared.GetVariableEntity(orgName, envName)
	if err != nil {
		return err
	}

	variableApp := shared.App(shared.Actions)
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

	updatec := make(chan updateResult)
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
			updatec <- updateVariable(opts, host, client, baseRepo, variableKey, variable.Name, variable.Value, visibility, repoIds, variableApp, variableEntity)
		}(variableKey, variable)
	}

	err = nil
	cs := opts.IO.ColorScheme()
	for i := 0; i < len(variables); i++ {
		result := <-updatec
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
		fmt.Fprintf(opts.IO.Out, "%s Update %s variable %s for %s\n", cs.SuccessIcon(), variableApp.Title(), result.key, target)
	}
	return err
}

type updateResult struct {
	key   string
	value string
	err   error
}

func updateVariable(opts *shared.PostPatchOptions, host string, client *api.Client, baseRepo ghrepo.Interface, variableKey, updatedVariableKey, variableValue, visibility string, repositoryIDs []int64, app shared.App, entity shared.VariableEntity) (res updateResult) {
	orgName := opts.OrgName
	envName := opts.EnvName
	res.key = updatedVariableKey
	var err error
	switch entity {
	case shared.Organization:
		err = patchOrgVariable(client, host, orgName, visibility, variableKey, updatedVariableKey, variableValue, repositoryIDs, app)
	case shared.Environment:
		err = patchEnvVariable(client, baseRepo, envName, variableKey, updatedVariableKey, variableValue)
	default:
		err = patchRepoVariable(client, baseRepo, variableKey, updatedVariableKey, variableValue, app)
	}
	if err != nil {
		res.err = fmt.Errorf("failed to update variable %q: %w", variableKey, err)
		return
	}
	return
}
