package remove

import (
	"fmt"
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type RemoveOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	Config     func() (config.Config, error)
	BaseRepo   func() (ghrepo.Interface, error)

	SecretName  string
	OrgName     string
	EnvName     string
	UserSecrets bool
}

func NewCmdRemove(f *cmdutil.Factory, runF func(*RemoveOptions) error) *cobra.Command {
	opts := &RemoveOptions{
		IO:         f.IOStreams,
		Config:     f.Config,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:   "remove <secret-name>",
		Short: "Remove secrets",
		Long: heredoc.Doc(`
			Remove a secret on one of the following levels:
			- repository (default): available to Actions runs in a repository
			- environment: available to Actions runs for a deployment environment in a repository
			- organization: available to Actions runs within an organization
			- user: available to Codespaces for your user
		`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if err := cmdutil.MutuallyExclusive("specify only one of `--org`, `--env`, or `--user`", opts.OrgName != "", opts.EnvName != "", opts.UserSecrets); err != nil {
				return err
			}

			opts.SecretName = args[0]

			if runF != nil {
				return runF(opts)
			}

			return removeRun(opts)
		},
	}
	cmd.Flags().StringVarP(&opts.OrgName, "org", "o", "", "Remove a secret for an organization")
	cmd.Flags().StringVarP(&opts.EnvName, "env", "e", "", "Remove a secret for an environment")
	cmd.Flags().BoolVarP(&opts.UserSecrets, "user", "u", false, "Remove a secret for your user")

	return cmd
}

func removeRun(opts *RemoveOptions) error {
	c, err := opts.HttpClient()
	if err != nil {
		return fmt.Errorf("could not create http client: %w", err)
	}
	client := api.NewClientFromHTTP(c)

	orgName := opts.OrgName
	envName := opts.EnvName

	var baseRepo ghrepo.Interface
	if orgName == "" && !opts.UserSecrets {
		baseRepo, err = opts.BaseRepo()
		if err != nil {
			return fmt.Errorf("could not determine base repo: %w", err)
		}
	}

	var path string
	if orgName != "" {
		path = fmt.Sprintf("orgs/%s/actions/secrets/%s", orgName, opts.SecretName)
	} else if envName != "" {
		path = fmt.Sprintf("repos/%s/environments/%s/secrets/%s", ghrepo.FullName(baseRepo), envName, opts.SecretName)
	} else if opts.UserSecrets {
		path = fmt.Sprintf("user/codespaces/secrets/%s", opts.SecretName)
	} else {
		path = fmt.Sprintf("repos/%s/actions/secrets/%s", ghrepo.FullName(baseRepo), opts.SecretName)
	}

	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	host, err := cfg.DefaultHost()
	if err != nil {
		return err
	}

	err = client.REST(host, "DELETE", path, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to delete secret %s: %w", opts.SecretName, err)
	}

	if opts.IO.IsStdoutTTY() {
		target := orgName
		if opts.UserSecrets {
			target = "your user"
		} else if orgName == "" {
			target = ghrepo.FullName(baseRepo)
		}
		cs := opts.IO.ColorScheme()
		if envName != "" {
			fmt.Fprintf(opts.IO.Out, "%s Removed secret %s from %s environment on %s\n", cs.SuccessIconWithColor(cs.Red), opts.SecretName, envName, target)
		} else {
			fmt.Fprintf(opts.IO.Out, "%s Removed secret %s from %s\n", cs.SuccessIconWithColor(cs.Red), opts.SecretName, target)
		}
	}

	return nil
}
