package view

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
	"github.com/cli/cli/v2/pkg/markdown"
	"github.com/cli/cli/v2/utils"

	"github.com/spf13/cobra"
)

type ViewOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (config.Config, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Browser    cmdutil.Browser

	Ref string
	Raw bool
	Web bool
}

func NewCmdView(f *cmdutil.Factory, runF func(*ViewOptions) error) *cobra.Command {
	opts := ViewOptions{
		HttpClient: f.HttpClient,
		Config:     f.Config,
		IO:         f.IOStreams,
		Browser:    f.Browser,
	}

	cmd := &cobra.Command{
		Use:   "view",
		Short: "View Dependabot configuration",
		Long: heredoc.Doc(`
			View the Dependabot configuration for a repository
		`),
		RunE: func(c *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if !opts.IO.IsStdoutTTY() {
				opts.Raw = true
			}

			if runF != nil {
				return runF(&opts)
			}
			return viewRun(&opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Ref, "ref", "r", "", "The branch or tag name which contains the version of the configuration file you'd like to view")
	cmd.Flags().BoolVarP(&opts.Raw, "raw", "", false, "Print raw instead of rendered gist contents")
	cmd.Flags().BoolVarP(&opts.Web, "web", "w", false, "Open configuration in the browser")

	return cmd
}

func viewRun(opts *ViewOptions) error {
	http, err := opts.HttpClient()
	if err != nil {
		return fmt.Errorf("could not build http client: %w", err)
	}
	client := api.NewClientFromHTTP(http)

	repo, err := opts.BaseRepo()
	if err != nil {
		return fmt.Errorf("could not determine base repo: %w", err)
	}

	ref := opts.Ref
	if ref == "" {
		opts.IO.StartProgressIndicator()
		ref, err = api.RepoDefaultBranch(client, repo)
		opts.IO.StopProgressIndicator()
		if err != nil {
			return fmt.Errorf("could not determine default branch for repo: %w", err)
		}
	}

	if opts.Web {
		url := dependabot.GenerateDependabotConfigurationURL(repo, ref, opts.Raw)

		if opts.IO.IsStdoutTTY() {
			fmt.Fprintf(opts.IO.Out, "Opening %s in your browser.\n", utils.DisplayURL(url))
		}

		return opts.Browser.Browse(url)
	}

	opts.IO.StartProgressIndicator()
	data, err := dependabot.FetchDependabotConfiguration(client, repo, ref)
	opts.IO.StopProgressIndicator()
	if err != nil {
		return err
	}

	if opts.Raw {
		opts.IO.Out.Write(data)
		return nil
	}

	opts.IO.DetectTerminalTheme()
	if err := opts.IO.StartPager(); err == nil {
		defer opts.IO.StopPager()
	} else {
		fmt.Fprintf(opts.IO.ErrOut, "failed to start pager: %v\n", err)
	}

	codeBlock := fmt.Sprintf("```yaml\n%s\n```", string(data))
	rendered, err := markdown.Render(codeBlock, markdown.WithIO(opts.IO), markdown.WithoutIndentation(), markdown.WithWrap(0))
	if err != nil {
		return err
	}

	_, err = fmt.Fprint(opts.IO.Out, rendered)
	return err
}
