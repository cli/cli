package clone

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type CloneOptions struct {
	HttpClient func() (*http.Client, error)
	GitClient  *git.Client
	Config     func() (gh.Config, error)
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
			them after %[1]s--%[1]s.

			If the %[1]sOWNER/%[1]s portion of the %[1]sOWNER/REPO%[1]s repository argument is omitted, it
			defaults to the name of the authenticating user.

			When a protocol scheme is not provided in the repository argument, the %[1]sgit_protocol%[1]s will be
			chosen from your configuration, which can be checked via %[1]sgh config get git_protocol%[1]s. If the protocol
			scheme is provided, the repository will be cloned using the specified protocol.

			If the repository is a fork, its parent repository will be added as an additional
			git remote called %[1]supstream%[1]s. The remote name can be configured using %[1]s--upstream-remote-name%[1]s.
			The %[1]s--upstream-remote-name%[1]s option supports an %[1]s@owner%[1]s value which will name
			the remote after the owner of the parent repository.

			If the repository is a fork, its parent repository will be set as the default remote repository.
		`, "`"),
		Example: heredoc.Doc(`
			# Clone a repository from a specific org
			$ gh repo clone cli/cli

			# Clone a repository from your own account
			$ gh repo clone myrepo

			# Clone a repo, overriding git protocol configuration
			$ gh repo clone https://github.com/cli/cli
			$ gh repo clone git@github.com:cli/cli.git

			# Clone a repository to a custom directory
			$ gh repo clone cli/cli workspace/cli

			# Clone a repository with additional git clone flags
			$ gh repo clone cli/cli -- --depth=1
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
		initialURL, err := git.ParseURL(opts.Repository)
		if err != nil {
			return err
		}

		repoURL := simplifyURL(initialURL)
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

		protocol = cfg.GitProtocol(repo.RepoHost()).Value
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

	// If the repo is a fork, add the parent as an upstream remote and set the parent as the default repo.
	if canonicalRepo.Parent != nil {
		protocol := cfg.GitProtocol(canonicalRepo.Parent.RepoHost()).Value
		upstreamURL := ghrepo.FormatRemoteURL(canonicalRepo.Parent, protocol)

		upstreamName := opts.UpstreamName
		if opts.UpstreamName == "@owner" {
			upstreamName = canonicalRepo.Parent.RepoOwner()
		}

		gc := gitClient.Copy()
		gc.RepoDir = cloneDir

		if _, err := gc.AddRemote(ctx, upstreamName, upstreamURL, []string{canonicalRepo.Parent.DefaultBranchRef.Name}); err != nil {
			return err
		}

		if err := gc.Fetch(ctx, upstreamName, ""); err != nil {
			return err
		}

		if err := gc.SetRemoteBranches(ctx, upstreamName, `*`); err != nil {
			return err
		}

		if err = gc.SetRemoteResolution(ctx, upstreamName, "base"); err != nil {
			return err
		}

		connectedToTerminal := opts.IO.IsStdoutTTY()
		if connectedToTerminal {
			cs := opts.IO.ColorScheme()
			fmt.Fprintf(opts.IO.ErrOut, "%s Repository %s set as the default repository. To learn more about the default repository, run: gh repo set-default --help\n", cs.WarningIcon(), cs.Bold(ghrepo.FullName(canonicalRepo.Parent)))
		}
	}
	return nil
}

// simplifyURL strips given URL of extra parts like extra path segments (i.e.,
// anything beyond `/owner/repo`), query strings, or fragments. This function
// never returns an error.
//
// The rationale behind this function is to let users clone a repo with any
// URL related to the repo; like:
//   - (Tree)              github.com/owner/repo/blob/main/foo/bar
//   - (Deep-link to line) github.com/owner/repo/blob/main/foo/bar#L168
//   - (Issue/PR comment)  github.com/owner/repo/pull/999#issue-9999999999
//   - (Commit history)    github.com/owner/repo/commits/main/?author=foo
func simplifyURL(u *url.URL) *url.URL {
	result := &url.URL{
		Scheme: u.Scheme,
		User:   u.User,
		Host:   u.Host,
		Path:   u.Path,
	}

	pathParts := strings.SplitN(strings.Trim(u.Path, "/"), "/", 3)
	if len(pathParts) <= 2 {
		return result
	}

	result.Path = strings.Join(pathParts[0:2], "/")
	return result
}
