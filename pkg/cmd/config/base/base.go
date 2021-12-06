package base

import (
	"fmt"
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type BaseOptions struct {
	IO         *iostreams.IOStreams
	Remotes    func() (context.Remotes, error)
	BaseRepo   func() (ghrepo.Interface, error)
	HttpClient func() (*http.Client, error)

	RemoteName string
	ViewFlag   bool
}

func NewCmdConfigBase(f *cmdutil.Factory, runF func(*BaseOptions) error) *cobra.Command {
	opts := &BaseOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		BaseRepo:   f.BaseRepo,
		Remotes:    f.Remotes,
	}

	cmd := &cobra.Command{
		Use:   "base <git remote name>",
		Short: "Configure the base repository used for various commands",
		Long: heredoc.Doc(`
		The base repository is used to determine which repository gh
		should automatically query for the commands:
			issue, pr, browse, run, repo rename, secret, workflow
		`),
		Example: heredoc.Doc(`
			$ gh base upstream
		`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.RemoteName = args[0]
			}
			if err := cmdutil.MutuallyExclusive(
				"args cannot be passed in with --view",
				opts.RemoteName != "",
				opts.ViewFlag,
			); err != nil {
				return err
			}

			if runF != nil {
				return runF(opts)
			}
			return baseRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.ViewFlag, "view", "v", false, "View the current configured default repository")
	return cmd
}

func baseRun(opts *BaseOptions) error {
	remotes, err := opts.Remotes()
	if err != nil {
		return err
	}

	if opts.ViewFlag {
		for _, remote := range remotes {
			if remote.Resolved == "base" {
				fmt.Println(remote.Remote.Name)
				return nil
			}
		}
		return fmt.Errorf("base repo has not been specified yet")
	}

	if opts.RemoteName != "" {
		for _, remote := range remotes {
			if opts.RemoteName == remote.Remote.Name {
				removeBaseRepo(remotes)
				git.SetRemoteResolution(remote.Name, "base")
				return nil
			}
		}
		return fmt.Errorf("could not find local remote name %s", opts.RemoteName)
	}
	removeBaseRepo(remotes)
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	apiClient := api.NewClientFromHTTP(httpClient)
	repoContext, err := context.ResolveRemotesToRepos(remotes, apiClient, "")
	if err != nil {
		return err
	}
	_, err = repoContext.BaseRepo(opts.IO)
	return err
}

func removeBaseRepo(remotes context.Remotes) {
	for _, remote := range remotes {
		if remote.Resolved == "base" {
			remote.Resolved = ""
			git.RemoveRemoteResolution(remote.Remote.Name)
		}
	}
}
