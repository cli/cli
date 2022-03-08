package base

import (
	"fmt"
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type DefaultOptions struct {
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Remotes    func() (context.Remotes, error)
	HttpClient func() (*http.Client, error)

	ViewFlag bool
}

func NewCmdDefault(f *cmdutil.Factory, runF func(*DefaultOptions) error) *cobra.Command {
	opts := &DefaultOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		BaseRepo:   f.BaseRepo,
		Remotes:    f.Remotes,
	}

	cmd := &cobra.Command{
		Use:   "default",
		Short: "Configure the default repository used for various commands",
		Long: heredoc.Doc(`
		The default repository is used to determine which repository gh
		should automatically query for the commands:
			issue, pr, browse, run, repo rename, secret, workflow
		`),
		Example: heredoc.Doc(`
			$ gh repo default cli/cli
		`),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !opts.IO.CanPrompt() && !opts.ViewFlag {
				return cmdutil.FlagErrorf("a repository name is required when not running interactively")
			}

			if runF != nil {
				return runF(opts)
			}
			return defaultRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.ViewFlag, "view", "v", false, "view the default repository used for various commands")
	return cmd
}

func defaultRun(opts *DefaultOptions) error {
	remotes, err := opts.Remotes()
	if err != nil {
		return err
	}

	if opts.ViewFlag {
		baseRepo, err := context.GetBaseRepo(remotes)
		if err != nil {
			return err
		}
		fmt.Fprintln(opts.IO.Out, ghrepo.FullName(baseRepo))
		return nil
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	apiClient := api.NewClientFromHTTP(httpClient)
	context.RemoveBaseRepo(remotes)
	repoContext, err := context.ResolveRemotesToRepos(remotes, apiClient, "")
	if err != nil {
		return err
	}
	return repoContext.SetGitConfigBaseRepo(opts.IO)
}
