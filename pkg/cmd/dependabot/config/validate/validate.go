package validate

import (
	"fmt"
	"io"
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	dependabot "github.com/cli/cli/v2/pkg/cmd/dependabot/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"

	"github.com/invopop/yaml"
	"github.com/spf13/cobra"
)

type ValidateOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (config.Config, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)

	Ref string
}

func NewCmdValidate(f *cmdutil.Factory, runF func(*ValidateOptions) error) *cobra.Command {
	opts := ValidateOptions{
		HttpClient: f.HttpClient,
		Config:     f.Config,
		IO:         f.IOStreams,
	}

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validates Dependabot configuration for a repository",
		Long: heredoc.Doc(`
			Validates the Dependabot configuration for a repository,
			By default, this is defined by the .github/dependabot.yml file
			located in the current repository.

            You can also validate configuration file locally by passing
            its contents to standard input.
		`),
		Example: heredoc.Doc(`
			# validate the Dependabot configuration for the current repository
			$ gh dependabot config validate
			# validate the Dependabot configuration for the octocat/hello repository
			$ gh dependabot config validate --repo octocat/hello
            # validate local configuration file
            $ cat .github/dependabot.yml | gh dependabot config validate
		`),
		RunE: func(c *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if runF != nil {
				return runF(&opts)
			}
			return validateRun(&opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Ref, "ref", "r", "", "The branch or tag name which contains the version of the configuration file you'd like to view")

	return cmd
}

func validateRun(opts *ValidateOptions) error {
	var data []byte
	var err error

	repo, err := opts.BaseRepo()
	if err != nil {
		return fmt.Errorf("could not determine base repo: %w", err)
	}

	if opts.IO.IsStdinTTY() {
		http, err := opts.HttpClient()
		if err != nil {
			return fmt.Errorf("could not build http client: %w", err)
		}
		client := api.NewClientFromHTTP(http)

		ref := opts.Ref
		if ref == "" {
			opts.IO.StartProgressIndicator()
			ref, err = api.RepoDefaultBranch(client, repo)
			opts.IO.StopProgressIndicator()
			if err != nil {
				return fmt.Errorf("could not determine default branch for repo: %w", err)
			}
		}

		opts.IO.StartProgressIndicator()
		data, err = dependabot.FetchDependabotConfiguration(client, repo, ref)
		opts.IO.StopProgressIndicator()
		if err != nil {
			return err
		}
	} else {
		data, err = io.ReadAll(opts.IO.In)
		if err != nil {
			return fmt.Errorf("could not read configuration: %w", err)
		}
	}

	data, err = yaml.YAMLToJSON(data)
	if err != nil {
		return err
	}

	if err := dependabot.ValidateDependabotConfiguration(data); err != nil {
		return err
	}

	cs := opts.IO.ColorScheme()
	if opts.IO.IsStdoutTTY() {
		fmt.Fprintf(opts.IO.Out, "%s Valid Dependabot configuration for %s\n", cs.SuccessIcon(), ghrepo.FullName(repo))
	}

	return nil
}
