package remove

import (
	"fmt"
	"net/http"

	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

type RemoveOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	Config     func() (config.Config, error)
	BaseRepo   func() (ghrepo.Interface, error)

	SecretName string
	OrgName    string
	EnvName    string
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
		Long:  "Remove a secret for a repository, environment, or organization",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if err := cmdutil.MutuallyExclusive("specify only one of `--org` or `--env`", opts.OrgName != "", opts.EnvName != ""); err != nil {
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
	if orgName == "" {
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
		if orgName == "" {
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
