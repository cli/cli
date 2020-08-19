package clone

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/api"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/internal/ghinstance"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type CloneOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (config.Config, error)
	IO         *iostreams.IOStreams

	GitArgs    []string
	Directory  string
	Repository string
}

func NewCmdClone(f *cmdutil.Factory, runF func(*CloneOptions) error) *cobra.Command {
	opts := &CloneOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
	}

	cmd := &cobra.Command{
		DisableFlagsInUseLine: true,

		Use:   "clone <repository> [<directory>] [-- <gitflags>...]",
		Args:  cobra.MinimumNArgs(1),
		Short: "Clone a repository locally",
		Long: heredoc.Doc(`
			Clone a GitHub repository locally.

			If the "OWNER/" portion of the "OWNER/REPO" repository argument is omitted, it
			defaults to the name of the authenticating user.
			
			Pass additional 'git clone' flags by listing them after '--'.
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Repository = args[0]
			opts.GitArgs = args[1:]

			if runF != nil {
				return runF(opts)
			}

			return cloneRun(opts)
		},
	}

	cmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		if err == pflag.ErrHelp {
			return err
		}
		return &cmdutil.FlagError{Err: fmt.Errorf("%w\nSeparate git clone flags with '--'.", err)}
	})

	return cmd
}

func cloneRun(opts *CloneOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	apiClient := api.NewClientFromHTTP(httpClient)
	cloneURL := opts.Repository
	if !strings.Contains(cloneURL, ":") {
		if !strings.Contains(cloneURL, "/") {
			currentUser, err := api.CurrentLoginName(apiClient, ghinstance.OverridableDefault())
			if err != nil {
				return err
			}
			cloneURL = currentUser + "/" + cloneURL
		}
		repo, err := ghrepo.FromFullName(cloneURL)
		if err != nil {
			return err
		}

		protocol, err := cfg.Get(repo.RepoHost(), "git_protocol")
		if err != nil {
			return err
		}
		cloneURL = ghrepo.FormatRemoteURL(repo, protocol)
	}

	var repo ghrepo.Interface
	var parentRepo ghrepo.Interface

	// TODO: consider caching and reusing `git.ParseSSHConfig().Translator()`
	// here to handle hostname aliases in SSH remotes
	if u, err := git.ParseURL(cloneURL); err == nil {
		repo, _ = ghrepo.FromURL(u)
	}

	if repo != nil {
		parentRepo, err = api.RepoParent(apiClient, repo)
		if err != nil {
			return err
		}
	}

	cloneDir, err := git.RunClone(cloneURL, opts.GitArgs)
	if err != nil {
		return err
	}

	if parentRepo != nil {
		protocol, err := cfg.Get(parentRepo.RepoHost(), "git_protocol")
		if err != nil {
			return err
		}
		upstreamURL := ghrepo.FormatRemoteURL(parentRepo, protocol)

		err = git.AddUpstreamRemote(upstreamURL, cloneDir)
		if err != nil {
			return err
		}
	}

	return nil
}
