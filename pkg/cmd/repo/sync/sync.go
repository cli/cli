package sync

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"

	"github.com/AlecAivazis/survey/v2"
	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/api"
	"github.com/cli/cli/context"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/prompt"
	"github.com/spf13/cobra"
)

type SyncOptions struct {
	HttpClient    func() (*http.Client, error)
	IO            *iostreams.IOStreams
	BaseRepo      func() (ghrepo.Interface, error)
	Remotes       func() (context.Remotes, error)
	CurrentBranch func() (string, error)
	Git           gitClient
	DestArg       string
	SrcArg        string
	Branch        string
	Force         bool
	SkipConfirm   bool
}

func NewCmdSync(f *cmdutil.Factory, runF func(*SyncOptions) error) *cobra.Command {
	opts := SyncOptions{
		HttpClient:    f.HttpClient,
		IO:            f.IOStreams,
		BaseRepo:      f.BaseRepo,
		Remotes:       f.Remotes,
		CurrentBranch: f.Branch,
		Git:           &gitExecuter{gitCommand: git.GitCommand},
	}

	cmd := &cobra.Command{
		Use:   "sync [<destination-repository>]",
		Short: "Sync a repository",
		Long: heredoc.Doc(`
			Sync destination repository from source repository.

			Without an argument, the local repository is selected as the destination repository.
			By default the source repository is the parent of the destination repository.
			The source repository can be overridden with the --source flag.
		`),
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
			if !opts.IO.CanPrompt() && !opts.SkipConfirm {
				return &cmdutil.FlagError{Err: errors.New("`--confirm` required when not running interactively")}
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
	cmd.Flags().BoolVarP(&opts.SkipConfirm, "confirm", "y", false, "Skip the confirmation prompt")
	return cmd
}

func syncRun(opts *SyncOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	apiClient := api.NewClientFromHTTP(httpClient)

	var local bool
	var destRepo, srcRepo ghrepo.Interface

	if opts.DestArg == "" {
		local = true
		destRepo, err = opts.BaseRepo()
		if err != nil {
			return err
		}
	} else {
		destRepo, err = ghrepo.FromFullName(opts.DestArg)
		if err != nil {
			return err
		}
	}

	if opts.SrcArg == "" {
		if local {
			srcRepo = destRepo
		} else {
			opts.IO.StartProgressIndicator()
			srcRepo, err = api.RepoParent(apiClient, destRepo)
			opts.IO.StopProgressIndicator()
			if err != nil {
				return err
			}
			if srcRepo == nil {
				return fmt.Errorf("can't determine source repo for %s because repo is not fork", ghrepo.FullName(destRepo))
			}
		}
	} else {
		srcRepo, err = ghrepo.FromFullName(opts.SrcArg)
		if err != nil {
			return err
		}
	}

	if !local && destRepo.RepoHost() != srcRepo.RepoHost() {
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

	srcStr := fmt.Sprintf("%s:%s", ghrepo.FullName(srcRepo), opts.Branch)
	destStr := fmt.Sprintf("%s:%s", ghrepo.FullName(destRepo), opts.Branch)
	if local {
		destStr = fmt.Sprintf(".:%s", opts.Branch)
	}
	cs := opts.IO.ColorScheme()
	if !opts.SkipConfirm && opts.IO.CanPrompt() {
		if opts.Force {
			fmt.Fprintf(opts.IO.ErrOut, "%s Using --force will cause diverging commits on %s to be discarded\n", cs.WarningIcon(), destStr)
		}
		var confirmed bool
		confirmQuestion := &survey.Confirm{
			Message: fmt.Sprintf("Sync %s from %s?", destStr, srcStr),
			Default: false,
		}
		err := prompt.SurveyAskOne(confirmQuestion, &confirmed)
		if err != nil {
			return err
		}

		if !confirmed {
			return cmdutil.CancelError
		}
	}

	opts.IO.StartProgressIndicator()
	if local {
		err = syncLocalRepo(srcRepo, opts)
	} else {
		err = syncRemoteRepo(apiClient, destRepo, srcRepo, opts)
	}
	opts.IO.StopProgressIndicator()

	if err != nil {
		return err
	}

	if opts.IO.IsStdoutTTY() {
		success := cs.Bold(fmt.Sprintf("Synced %s from %s\n", destStr, srcStr))
		fmt.Fprintf(opts.IO.Out, "%s %s", cs.SuccessIconWithColor(cs.GreenBold), success)
	}

	return nil
}

func syncLocalRepo(srcRepo ghrepo.Interface, opts *SyncOptions) error {
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
			return fmt.Errorf("can't sync .:%s because there are diverging commits, try using `--force`", branch)
		}
	}

	dirtyRepo, err := git.IsDirty()
	if err != nil {
		return err
	}
	startBranch, err := git.CurrentBranch()
	if err != nil {
		return err
	}

	if dirtyRepo {
		err = git.Stash([]string{"push"})
		if err != nil {
			return err
		}
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
	if dirtyRepo {
		err = git.Stash([]string{"pop"})
		if err != nil {
			return err
		}
	}

	return nil
}

func syncRemoteRepo(client *api.Client, destRepo, srcRepo ghrepo.Interface, opts *SyncOptions) error {
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
		return fmt.Errorf("can't sync %s:%s because there are diverging commits, try using `--force`",
			ghrepo.FullName(destRepo),
			opts.Branch)
	}

	return err
}
