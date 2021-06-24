package sync

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/api"
	"github.com/cli/cli/context"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

type SyncOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Remotes    func() (context.Remotes, error)
	Git        gitClient
	DestArg    string
	SrcArg     string
	Branch     string
	Force      bool
}

func NewCmdSync(f *cmdutil.Factory, runF func(*SyncOptions) error) *cobra.Command {
	opts := SyncOptions{
		HttpClient: f.HttpClient,
		IO:         f.IOStreams,
		BaseRepo:   f.BaseRepo,
		Remotes:    f.Remotes,
		Git:        &gitExecuter{},
	}

	cmd := &cobra.Command{
		Use:   "sync [<destination-repository>]",
		Short: "Sync a repository",
		Long: heredoc.Docf(`
			Sync destination repository from source repository. Syncing will take a branch
			on the source repository and merge it into the branch of the same name on the
			destination repository. A fast forward merge will be used execept when the
			%[1]s--force%[1]s flag is specified, then the two branches will by synced using
			a hard reset.

			Without an argument, the local repository is selected as the destination repository.

			By default the source repository is the parent of the destination repository,
			this can be overridden with the %[1]s--source%[1]s flag.

			The source repository default branch is selected automatically, but can be
			overridden with the %[1]s--branch%[1]s flag.
		`, "`"),
		Example: heredoc.Doc(`
			# Sync local repository from remote parent
			$ gh repo sync

			# Sync local repository from remote parent on non-default branch
			$ gh repo sync --branch v1

			# Sync remote fork from remote parent
			$ gh repo sync owner/cli-fork

			# Sync remote repo from another remote repo
			$ gh repo sync owner/repo --source owner2/repo2
		`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.DestArg = args[0]
			}
			if runF != nil {
				return runF(&opts)
			}
			return syncRun(&opts)
		},
	}

	cmd.Flags().StringVarP(&opts.SrcArg, "source", "s", "", "Source repository")
	cmd.Flags().StringVarP(&opts.Branch, "branch", "b", "", "Branch to sync")
	cmd.Flags().BoolVarP(&opts.Force, "force", "", false, "Discard destination repository changes")
	return cmd
}

func syncRun(opts *SyncOptions) error {
	if opts.DestArg == "" {
		return syncLocalRepo(opts)
	} else {
		return syncRemoteRepo(opts)
	}
}

func syncLocalRepo(opts *SyncOptions) error {
	var err error
	var srcRepo ghrepo.Interface

	if opts.SrcArg != "" {
		srcRepo, err = ghrepo.FromFullName(opts.SrcArg)
	} else {
		srcRepo, err = opts.BaseRepo()
	}
	if err != nil {
		return err
	}

	dirtyRepo, err := opts.Git.IsDirty()
	if err != nil {
		return err
	}
	if dirtyRepo {
		return fmt.Errorf("can't sync because there are local changes, please commit or stash them")
	}

	if opts.Branch == "" {
		httpClient, err := opts.HttpClient()
		if err != nil {
			return err
		}
		apiClient := api.NewClientFromHTTP(httpClient)
		opts.IO.StartProgressIndicator()
		opts.Branch, err = api.RepoDefaultBranch(apiClient, srcRepo)
		opts.IO.StopProgressIndicator()
		if err != nil {
			return err
		}
	}

	opts.IO.StartProgressIndicator()
	err = executeLocalRepoSync(srcRepo, opts)
	opts.IO.StopProgressIndicator()
	if err != nil {
		if errors.Is(err, divergingError) {
			return fmt.Errorf("can't sync because there are diverging changes, you can use `--force` to overwrite the changes")
		}
		return err
	}

	if opts.IO.IsStdoutTTY() {
		cs := opts.IO.ColorScheme()
		srcStr := fmt.Sprintf("%s:%s", ghrepo.FullName(srcRepo), opts.Branch)
		destStr := fmt.Sprintf(".:%s", opts.Branch)
		success := fmt.Sprintf("Synced %s from %s\n", cs.Bold(destStr), cs.Bold(srcStr))
		fmt.Fprintf(opts.IO.Out, "%s %s", cs.SuccessIcon(), success)
	}

	return nil
}

func syncRemoteRepo(opts *SyncOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	apiClient := api.NewClientFromHTTP(httpClient)

	var destRepo, srcRepo ghrepo.Interface

	destRepo, err = ghrepo.FromFullName(opts.DestArg)
	if err != nil {
		return err
	}

	if opts.SrcArg == "" {
		opts.IO.StartProgressIndicator()
		srcRepo, err = api.RepoParent(apiClient, destRepo)
		opts.IO.StopProgressIndicator()
		if err != nil {
			return err
		}
		if srcRepo == nil {
			return fmt.Errorf("can't determine source repo for %s because repo is not fork", ghrepo.FullName(destRepo))
		}
	} else {
		srcRepo, err = ghrepo.FromFullName(opts.SrcArg)
		if err != nil {
			return err
		}
	}

	if destRepo.RepoHost() != srcRepo.RepoHost() {
		return fmt.Errorf("can't sync repos from different hosts")
	}

	if opts.Branch == "" {
		opts.IO.StartProgressIndicator()
		opts.Branch, err = api.RepoDefaultBranch(apiClient, srcRepo)
		opts.IO.StopProgressIndicator()
		if err != nil {
			return err
		}
	}

	opts.IO.StartProgressIndicator()
	err = executeRemoteRepoSync(apiClient, destRepo, srcRepo, opts)
	opts.IO.StopProgressIndicator()
	if err != nil {
		if errors.Is(err, divergingError) {
			return fmt.Errorf("can't sync because there are diverging changes, you can use `--force` to overwrite the changes")
		}
		return err
	}

	if opts.IO.IsStdoutTTY() {
		cs := opts.IO.ColorScheme()
		srcStr := fmt.Sprintf("%s:%s", ghrepo.FullName(srcRepo), opts.Branch)
		destStr := fmt.Sprintf("%s:%s", ghrepo.FullName(destRepo), opts.Branch)
		success := fmt.Sprintf("Synced %s from %s\n", cs.Bold(destStr), cs.Bold(srcStr))
		fmt.Fprintf(opts.IO.Out, "%s %s", cs.SuccessIcon(), success)
	}

	return nil
}

var divergingError = errors.New("diverging changes")

func executeLocalRepoSync(srcRepo ghrepo.Interface, opts *SyncOptions) error {
	// Remotes precedence by name
	// 1. upstream
	// 2. github
	// 3. origin
	// 4. other
	remotes, err := opts.Remotes()
	if err != nil {
		return err
	}
	remote := remotes[0]
	branch := opts.Branch
	git := opts.Git

	err = git.Fetch([]string{remote.Name, fmt.Sprintf("+refs/heads/%s", branch)})
	if err != nil {
		return err
	}

	hasLocalBranch := git.HasLocalBranch([]string{branch})
	if hasLocalBranch {
		fastForward, err := git.IsAncestor([]string{branch, fmt.Sprintf("%s/%s", remote.Name, branch)})
		if err != nil {
			return err
		}

		if !fastForward && !opts.Force {
			return divergingError
		}
	}

	startBranch, err := git.CurrentBranch()
	if err != nil {
		return err
	}
	if startBranch != branch {
		err = git.Checkout([]string{branch})
		if err != nil {
			return err
		}
	}
	if hasLocalBranch {
		if opts.Force {
			err = git.Reset([]string{"--hard", fmt.Sprintf("refs/remotes/%s/%s", remote, branch)})
			if err != nil {
				return err
			}
		} else {
			err = git.Merge([]string{"--ff-only", fmt.Sprintf("refs/remotes/%s/%s", remote, branch)})
			if err != nil {
				return err
			}
		}
	}
	if startBranch != branch {
		err = git.Checkout([]string{startBranch})
		if err != nil {
			return err
		}
	}

	return nil
}

func executeRemoteRepoSync(client *api.Client, destRepo, srcRepo ghrepo.Interface, opts *SyncOptions) error {
	commit, err := latestCommit(client, srcRepo, opts.Branch)
	if err != nil {
		return err
	}

	// This is not a great way to detect the error returned by the API
	// Unfortunately API returns 422 for multiple reasons
	notFastForwardErrorMessage := regexp.MustCompile(`^Update is not a fast forward$`)
	err = syncFork(client, destRepo, opts.Branch, commit.Object.SHA, opts.Force)
	var httpErr api.HTTPError
	if err != nil && errors.As(err, &httpErr) && notFastForwardErrorMessage.MatchString(httpErr.Message) {
		return divergingError
	}

	return err
}
