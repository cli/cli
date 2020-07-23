package clone

import (
	"net/http"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/api"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/spf13/cobra"
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
		Use:   "clone <repository> [<directory>]",
		Args:  cobra.MinimumNArgs(1),
		Short: "Clone a repository locally",
		Long: heredoc.Doc(
			`Clone a GitHub repository locally.

			If the "OWNER/" portion of the "OWNER/REPO" repository argument is omitted, it
			defaults to the name of the authenticating user.
			
			To pass 'git clone' flags, separate them with '--'.
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

	return cmd
}

func cloneRun(opts *CloneOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	// TODO This is overly wordy and I'd like to streamline this.
	cfg, err := opts.Config()
	if err != nil {
		return err
	}
	protocol, err := cfg.Get("", "git_protocol")
	if err != nil {
		return err
	}

	apiClient := api.NewClientFromHTTP(httpClient)
	cloneURL := opts.Repository
	if !strings.Contains(cloneURL, ":") {
		if !strings.Contains(cloneURL, "/") {
			currentUser, err := api.CurrentLoginName(apiClient)
			if err != nil {
				return err
			}
			cloneURL = currentUser + "/" + cloneURL
		}
		repo, err := ghrepo.FromFullName(cloneURL)
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
		upstreamURL := ghrepo.FormatRemoteURL(parentRepo, protocol)

		err := git.AddUpstreamRemote(upstreamURL, cloneDir)
		if err != nil {
			return err
		}
	}

	return nil
}
