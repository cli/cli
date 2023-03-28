package sync

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/context"
	gitpkg "github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

const (
	notFastForwardErrorMessage     = "Update is not a fast forward"
	branchDoesNotExistErrorMessage = "Reference does not exist"
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
		Git:        &gitExecuter{client: f.GitClient},
	}

	cmd := &cobra.Command{
		Use:   "sync [<destination-repository>]",
		Short: "Sync a repository",
		Long: heredoc.Docf(`
			Sync destination repository from source repository. Syncing uses the main branch
			of the source repository to update the matching branch on the destination
			repository so they are equal. A fast forward update will be used except when the
			%[1]s--force%[1]s flag is specified, then the two branches will
			by synced using a hard reset.

			Without an argument, the local repository is selected as the destination repository.

			The source repository is the parent of the destination repository by default.
			This can be overridden with the %[1]s--source%[1]s flag.
		`, "`"),
		Example: heredoc.Doc(`
			# Sync local repository from remote parent
			$ gh repo sync

			# Sync local repository from remote parent on specific branch
			$ gh repo sync --branch v1

			# Sync remote fork from its parent
			$ gh repo sync owner/cli-fork

			# Sync remote repository from another remote repository
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
	cmd.Flags().StringVarP(&opts.Branch, "branch", "b", "", "Branch to sync (default: main branch)")
	cmd.Flags().BoolVarP(&opts.Force, "force", "", false, "Hard reset the branch of the destination repository to match the source repository")
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
	var srcRepo ghrepo.Interface

	if opts.SrcArg != "" {
		var err error
		srcRepo, err = ghrepo.FromFullName(opts.SrcArg)
		if err != nil {
			return err
		}
	} else {
		var err error
		srcRepo, err = opts.BaseRepo()
		if err != nil {
			return err
		}
	}

	// Find remote that matches the srcRepo
	var remote string
	remotes, err := opts.Remotes()
	if err != nil {
		return err
	}
	if r, err := remotes.FindByRepo(srcRepo.RepoOwner(), srcRepo.RepoName()); err == nil {
		remote = r.Name
	} else {
		return fmt.Errorf("can't find corresponding remote for %s", ghrepo.FullName(srcRepo))
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

	// Git fetch might require input from user, so do it before starting progress indicator.
	if err := opts.Git.Fetch(remote, fmt.Sprintf("refs/heads/%s", opts.Branch)); err != nil {
		return err
	}

	opts.IO.StartProgressIndicator()
	err = executeLocalRepoSync(srcRepo, remote, opts)
	opts.IO.StopProgressIndicator()
	if err != nil {
		if errors.Is(err, divergingError) {
			return fmt.Errorf("can't sync because there are diverging changes; use `--force` to overwrite the destination branch")
		}
		if errors.Is(err, mismatchRemotesError) {
			return fmt.Errorf("can't sync because %s is not tracking %s", opts.Branch, ghrepo.FullName(srcRepo))
		}
		return err
	}

	if opts.IO.IsStdoutTTY() {
		cs := opts.IO.ColorScheme()
		fmt.Fprintf(opts.IO.Out, "%s Synced the \"%s\" branch from %s to local repository\n",
			cs.SuccessIcon(),
			opts.Branch,
			ghrepo.FullName(srcRepo))
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

	if opts.SrcArg != "" {
		srcRepo, err = ghrepo.FromFullName(opts.SrcArg)
		if err != nil {
			return err
		}
	}

	if srcRepo != nil && destRepo.RepoHost() != srcRepo.RepoHost() {
		return fmt.Errorf("can't sync repositories from different hosts")
	}

	opts.IO.StartProgressIndicator()
	baseBranchLabel, err := executeRemoteRepoSync(apiClient, destRepo, srcRepo, opts)
	opts.IO.StopProgressIndicator()
	if err != nil {
		if errors.Is(err, divergingError) {
			return fmt.Errorf("can't sync because there are diverging changes; use `--force` to overwrite the destination branch")
		}
		return err
	}

	if opts.IO.IsStdoutTTY() {
		cs := opts.IO.ColorScheme()
		branchName := opts.Branch
		if idx := strings.Index(baseBranchLabel, ":"); idx >= 0 {
			branchName = baseBranchLabel[idx+1:]
		}
		fmt.Fprintf(opts.IO.Out, "%s Synced the \"%s:%s\" branch from \"%s\"\n",
			cs.SuccessIcon(),
			destRepo.RepoOwner(),
			branchName,
			baseBranchLabel)
	}

	return nil
}

var divergingError = errors.New("diverging changes")
var mismatchRemotesError = errors.New("branch remote does not match specified source")

func executeLocalRepoSync(srcRepo ghrepo.Interface, remote string, opts *SyncOptions) error {
	git := opts.Git
	branch := opts.Branch
	useForce := opts.Force

	hasLocalBranch := git.HasLocalBranch(branch)
	if hasLocalBranch {
		branchRemote, err := git.BranchRemote(branch)
		if err != nil {
			return err
		}
		if branchRemote != remote {
			return mismatchRemotesError
		}

		fastForward, err := git.IsAncestor(branch, "FETCH_HEAD")
		if err != nil {
			return err
		}

		if !fastForward && !useForce {
			return divergingError
		}
		if fastForward && useForce {
			useForce = false
		}
	}

	currentBranch, err := git.CurrentBranch()
	if err != nil && !errors.Is(err, gitpkg.ErrNotOnAnyBranch) {
		return err
	}
	if currentBranch == branch {
		if isDirty, err := git.IsDirty(); err == nil && isDirty {
			return fmt.Errorf("refusing to sync due to uncommitted/untracked local changes\ntip: use `git stash --all` before retrying the sync and run `git stash pop` afterwards")
		} else if err != nil {
			return err
		}
		if useForce {
			if err := git.ResetHard("FETCH_HEAD"); err != nil {
				return err
			}
		} else {
			if err := git.MergeFastForward("FETCH_HEAD"); err != nil {
				return err
			}
		}
	} else {
		if hasLocalBranch {
			if err := git.UpdateBranch(branch, "FETCH_HEAD"); err != nil {
				return err
			}
		} else {
			if err := git.CreateBranch(branch, "FETCH_HEAD", fmt.Sprintf("%s/%s", remote, branch)); err != nil {
				return err
			}
		}
	}

	return nil
}

func executeRemoteRepoSync(client *api.Client, destRepo, srcRepo ghrepo.Interface, opts *SyncOptions) (string, error) {
	branchName := opts.Branch
	if branchName == "" {
		var err error
		branchName, err = api.RepoDefaultBranch(client, destRepo)
		if err != nil {
			return "", err
		}
	}

	var apiErr upstreamMergeErr
	if baseBranch, err := triggerUpstreamMerge(client, destRepo, branchName); err == nil {
		return baseBranch, nil
	} else if !errors.As(err, &apiErr) {
		return "", err
	}

	if srcRepo == nil {
		var err error
		srcRepo, err = api.RepoParent(client, destRepo)
		if err != nil {
			return "", err
		}
		if srcRepo == nil {
			return "", fmt.Errorf("can't determine source repository for %s because repository is not fork", ghrepo.FullName(destRepo))
		}
	}

	commit, err := latestCommit(client, srcRepo, branchName)
	if err != nil {
		return "", err
	}

	// This is not a great way to detect the error returned by the API
	// Unfortunately API returns 422 for multiple reasons
	err = syncFork(client, destRepo, branchName, commit.Object.SHA, opts.Force)
	var httpErr api.HTTPError
	if err != nil {
		if errors.As(err, &httpErr) {
			switch httpErr.Message {
			case notFastForwardErrorMessage:
				return "", divergingError
			case branchDoesNotExistErrorMessage:
				return "", fmt.Errorf("%s branch does not exist on %s repository", branchName, ghrepo.FullName(destRepo))
			}
		}
		return "", err
	}

	return fmt.Sprintf("%s:%s", srcRepo.RepoOwner(), branchName), nil
}
