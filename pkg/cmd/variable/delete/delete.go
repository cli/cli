package delete

import (
	"fmt"
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	ghContext "github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/variable/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type DeleteOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	Config     func() (gh.Config, error)
	BaseRepo   func() (ghrepo.Interface, error)
	Remotes    func() (ghContext.Remotes, error)

	VariableName string
	OrgName      string
	EnvName      string

	HasRepoOverride bool
}

func NewCmdDelete(f *cmdutil.Factory, runF func(*DeleteOptions) error) *cobra.Command {
	opts := &DeleteOptions{
		IO:         f.IOStreams,
		Config:     f.Config,
		HttpClient: f.HttpClient,
		Remotes:    f.Remotes,
	}

	cmd := &cobra.Command{
		Use:   "delete <variable-name>",
		Short: "Delete variables",
		Long: heredoc.Doc(`
			Delete a variable on one of the following levels:
			- repository (default): available to GitHub Actions runs or Dependabot in a repository
			- environment: available to GitHub Actions runs for a deployment environment in a repository
			- organization: available to GitHub Actions runs or Dependabot within an organization
		`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if err := cmdutil.MutuallyExclusive("specify only one of `--org` or `--env`", opts.OrgName != "", opts.EnvName != ""); err != nil {
				return err
			}

			opts.VariableName = args[0]
			opts.HasRepoOverride = cmd.Flags().Changed("repo")

			if runF != nil {
				return runF(opts)
			}

			return removeRun(opts)
		},
		Aliases: []string{
			"remove",
		},
	}
	cmd.Flags().StringVarP(&opts.OrgName, "org", "o", "", "Delete a variable for an organization")
	cmd.Flags().StringVarP(&opts.EnvName, "env", "e", "", "Delete a variable for an environment")

	return cmd
}

func removeRun(opts *DeleteOptions) error {
	c, err := opts.HttpClient()
	if err != nil {
		return fmt.Errorf("could not create http client: %w", err)
	}
	client := api.NewClientFromHTTP(c)

	orgName := opts.OrgName
	envName := opts.EnvName

	variableEntity, err := shared.GetVariableEntity(orgName, envName)
	if err != nil {
		return err
	}

	var baseRepo ghrepo.Interface
	if variableEntity == shared.Repository || variableEntity == shared.Environment {
		baseRepo, err = opts.BaseRepo()
		if err != nil {
			return err
		}

		err = cmdutil.ValidateHasOnlyOneRemote(opts.HasRepoOverride, opts.Remotes)
		if err != nil {
			return err
		}
	}

	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	var path string
	var host string
	switch variableEntity {
	case shared.Organization:
		path = fmt.Sprintf("orgs/%s/actions/variables/%s", orgName, opts.VariableName)
		host, _ = cfg.Authentication().DefaultHost()
	case shared.Environment:
		path = fmt.Sprintf("repos/%s/environments/%s/variables/%s", ghrepo.FullName(baseRepo), envName, opts.VariableName)
		host = baseRepo.RepoHost()
	case shared.Repository:
		path = fmt.Sprintf("repos/%s/actions/variables/%s", ghrepo.FullName(baseRepo), opts.VariableName)
		host = baseRepo.RepoHost()
	}

	err = client.REST(host, "DELETE", path, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to delete variable %s: %w", opts.VariableName, err)
	}

	if opts.IO.IsStdoutTTY() {
		var target string
		switch variableEntity {
		case shared.Organization:
			target = orgName
		case shared.Repository, shared.Environment:
			target = ghrepo.FullName(baseRepo)
		}

		cs := opts.IO.ColorScheme()
		if envName != "" {
			fmt.Fprintf(opts.IO.Out, "%s Deleted variable %s from %s environment on %s\n", cs.SuccessIconWithColor(cs.Red), opts.VariableName, envName, target)
		} else {
			fmt.Fprintf(opts.IO.Out, "%s Deleted variable %s from %s\n", cs.SuccessIconWithColor(cs.Red), opts.VariableName, target)
		}
	}

	return nil
}
