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
		Args:  cmdutil.MinimumArgs(1, "cannot clone: repository argument required"),
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

	repositoryIsURL := strings.Contains(opts.Repository, ":")
	repositoryIsFullName := !repositoryIsURL && strings.Contains(opts.Repository, "/")

	var repo ghrepo.Interface
	var protocol string

	if repositoryIsURL {
		repoURL, err := git.ParseURL(opts.Repository)
		if err != nil {
			return err
		}
		repo, err = ghrepo.FromURL(repoURL)
		if err != nil {
			return err
		}
		if repoURL.Scheme == "git+ssh" {
			repoURL.Scheme = "ssh"
		}
		protocol = repoURL.Scheme
	} else {
		var fullName string
		if repositoryIsFullName {
			fullName = opts.Repository
		} else {
			currentUser, err := api.CurrentLoginName(apiClient, ghinstance.OverridableDefault())
			if err != nil {
				return err
			}
			fullName = currentUser + "/" + opts.Repository
		}

		repo, err = ghrepo.FromFullName(fullName)
		if err != nil {
			return err
		}

		protocol, err = cfg.Get(repo.RepoHost(), "git_protocol")
		if err != nil {
			return err
		}
	}

	wantsWiki := strings.HasSuffix(repo.RepoName(), ".wiki")
	if wantsWiki {
		repoName := strings.TrimSuffix(repo.RepoName(), ".wiki")
		repo = ghrepo.NewWithHost(repo.RepoOwner(), repoName, repo.RepoHost())
	}

	// Load the repo from the API to get the username/repo name in its
	// canonical capitalization
	canonicalRepo, err := api.GitHubRepo(apiClient, repo)
	if err != nil {
		return err
	}
	canonicalCloneURL := ghrepo.FormatRemoteURL(canonicalRepo, protocol)

	// If repo HasWikiEnabled and wantsWiki is true then create a new clone URL
	if wantsWiki {
		if !canonicalRepo.HasWikiEnabled {
			return fmt.Errorf("The '%s' repository does not have a wiki", ghrepo.FullName(canonicalRepo))
		}
		canonicalCloneURL = strings.TrimSuffix(canonicalCloneURL, ".git") + ".wiki.git"
	}

	cloneDir, err := git.RunClone(canonicalCloneURL, opts.GitArgs)
	if err != nil {
		return err
	}

	// If the repo is a fork, add the parent as an upstream
	if canonicalRepo.Parent != nil {
		protocol, err := cfg.Get(canonicalRepo.Parent.RepoHost(), "git_protocol")
		if err != nil {
			return err
		}
		upstreamURL := ghrepo.FormatRemoteURL(canonicalRepo.Parent, protocol)

		err = git.AddUpstreamRemote(upstreamURL, cloneDir)
		if err != nil {
			return err
		}
	}

	return nil
}
