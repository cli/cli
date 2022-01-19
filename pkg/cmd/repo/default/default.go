package base

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type DefaultOptions struct {
	IO         *iostreams.IOStreams
	Remotes    func() (context.Remotes, error)
	BaseRepo   func() (ghrepo.Interface, error)
	HttpClient func() (*http.Client, error)

	RemoteName string
	ListFlag   bool
}

func NewCmdDefault(f *cmdutil.Factory, runF func(*DefaultOptions) error) *cobra.Command {
	opts := &DefaultOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		BaseRepo:   f.BaseRepo,
		Remotes:    f.Remotes,
	}

	cmd := &cobra.Command{
		Use:   "default <git remote name>",
		Short: "Configure the default repository used for various commands",
		Long: heredoc.Doc(`
		The default repository is used to determine which repository gh
		should automatically query for the commands:
			issue, pr, browse, run, repo rename, secret, workflow
		`),
		Example: heredoc.Doc(`
			$ gh repo default upstream
		`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.RemoteName = args[0]
			}
			if !opts.IO.CanPrompt() && !opts.ListFlag && opts.RemoteName == "" {
				return cmdutil.FlagErrorf("remote name is required when not running interactively, use `--list` to see available remotes")
			}
			if err := cmdutil.MutuallyExclusive(
				"args cannot be passed in with --list",
				opts.RemoteName != "",
				opts.ListFlag,
			); err != nil {
				return err
			}

			if runF != nil {
				return runF(opts)
			}
			return defaultRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.ListFlag, "list", "l", false, "list the current and available repositories")
	return cmd
}

func defaultRun(opts *DefaultOptions) error {
	remotes, err := opts.Remotes()
	if err != nil {
		return err
	}

	if opts.ListFlag {
		found := false
		list := &strings.Builder{}
		for _, remote := range remotes {
			if remote.Resolved == "base" {
				list.WriteString(fmt.Sprintf("* %s\n", remote.Remote.Name))
				found = true
			} else {
				list.WriteString(fmt.Sprintf("  %s\n", remote.Remote.Name))
			}
		}
		if !found {
			fmt.Fprint(opts.IO.Out, "the default repo has not been set\n")
		}
		fmt.Fprint(opts.IO.Out, list.String())
		return nil
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
			git.UnsetRemoteResolution(remote.Remote.Name)
		}
	}
}
