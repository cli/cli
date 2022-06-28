package create

import (
	"fmt"
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	dependabot "github.com/cli/cli/v2/pkg/cmd/dependabot/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"

	"github.com/spf13/cobra"
)

type CreateOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (config.Config, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Browser    cmdutil.Browser
}

func NewCmdCreate(f *cmdutil.Factory, runF func(*CreateOptions) error) *cobra.Command {
	opts := CreateOptions{
		Browser:    f.Browser,
		HttpClient: f.HttpClient,
		Config:     f.Config,
		IO:         f.IOStreams,
	}

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Creates Dependabot configuration for a repository",
		Long: heredoc.Doc(`
			Creates the Dependabot configuration for a repository,
			By default, this is defined by the .github/dependabot.yml file
			located in the current repository on its default branch.
		`),
		Example: heredoc.Doc(`
			# create the Dependabot configuration for the current repository
			$ gh dependabot config create
			# create the Dependabot configuration for the octocat/hello repository
			$ gh dependabot config create --repo octocat/hello
		`),
		RunE: func(c *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if runF != nil {
				return runF(&opts)
			}
			return createRun(&opts)
		},
	}

	return cmd
}

func createRun(opts *CreateOptions) error {
	c, err := opts.HttpClient()
	if err != nil {
		return fmt.Errorf("could not build http client: %w", err)
	}
	client := api.NewClientFromHTTP(c)

	repo, err := opts.BaseRepo()
	if err != nil {
		return fmt.Errorf("could not determine base repo: %w", err)
	}

	opts.IO.StartProgressIndicator()
	ref, err := api.RepoDefaultBranch(client, repo)
	opts.IO.StopProgressIndicator()
	if err != nil {
		return err
	}

	_, err = dependabot.FetchDependabotConfiguration(client, repo, ref)
	if err == nil {
		return fmt.Errorf("configuration already exists on the default branch for %s", ghrepo.FullName(repo))
	}

	url := dependabot.GenerateDependabotConfigurationTemplateURL(repo, ref)
	if opts.IO.IsStdoutTTY() {
		fmt.Fprintf(opts.IO.ErrOut, "Opening dependabot.yml template for %s in your browser.\n", ghrepo.FullName(repo))
	}
	return opts.Browser.Browse(url)
}
