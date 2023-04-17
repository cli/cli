package clone

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type CloneOptions struct {
	HttpClient func() (*http.Client, error)
	GitClient  *git.Client
	Config     func() (config.Config, error)
	IO         *iostreams.IOStreams

	GitArgs      []string
	Repository   string
	UpstreamName string
}

func NewCmdClone(f *cmdutil.Factory, runF func(*CloneOptions) error) *cobra.Command {
	opts := &CloneOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		GitClient:  f.GitClient,
		Config:     f.Config,
	}

	cmd := &cobra.Command{
		DisableFlagsInUseLine: true,

		Use:   "clone <repository> [<directory>] [-- <gitflags>...]",
		Args:  cmdutil.MinimumArgs(1, "cannot clone: repository argument required"),
		Short: "Clone a repository locally",
		Long: heredoc.Docf(`
			Clone a GitHub repository locally. Pass additional %[1]sgit clone%[1]s flags by listing
			them after "--".

			If the "OWNER/" portion of the "OWNER/REPO" repository argument is omitted, it
			defaults to the name of the authenticating user.

			If the repository is a fork, its parent repository will be added as an additional
			git remote called "upstream". The remote name can be configured using %[1]s--upstream-remote-name%[1]s.
			The %[1]s--upstream-remote-name%[1]s option supports an "@owner" value which will name
			the remote after the owner of the parent repository.
		`, "`"),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Repository = args[0]
			opts.GitArgs = args[1:]

			if runF != nil {
				return runF(opts)
			}

			return cloneRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.UpstreamName, "upstream-remote-name", "u", "upstream", "Upstream remote name when cloning a fork")
	cmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		if err == pflag.ErrHelp {
			return err
		}
		return cmdutil.FlagErrorf("%w\nSeparate git clone flags with '--'.", err)
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
			host, _ := cfg.Authentication().DefaultHost()
			currentUser, err := api.CurrentLoginName(apiClient, host)
			if err != nil {
				return err
			}
			fullName = currentUser + "/" + opts.Repository
		}

		repo, err = ghrepo.FromFullName(fullName)
		if err != nil {
			return err
		}

		protocol, err = cfg.GetOrDefault(repo.RepoHost(), "git_protocol")
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

	gitClient := opts.GitClient
	ctx := context.Background()
	cloneDir, err := gitClient.Clone(ctx, canonicalCloneURL, opts.GitArgs)
	if err != nil {
		return err
	}

	// If the repo is a fork, add the parent as an upstream
	if canonicalRepo.Parent != nil {
		protocol, err := cfg.GetOrDefault(canonicalRepo.Parent.RepoHost(), "git_protocol")
		if err != nil {
			return err
		}
		upstreamURL := ghrepo.FormatRemoteURL(canonicalRepo.Parent, protocol)

		upstreamName := opts.UpstreamName
		if opts.UpstreamName == "@owner" {
			upstreamName = canonicalRepo.Parent.RepoOwner()
		}

		_, err = gitClient.AddRemote(ctx, upstreamName, upstreamURL, []string{canonicalRepo.Parent.DefaultBranchRef.Name}, git.WithRepoDir(cloneDir))
		if err != nil {
			return err
		}

		if err := gitClient.Fetch(ctx, upstreamName, "", git.WithRepoDir(cloneDir)); err != nil {
			return err
		}
	}
	return nil
}
