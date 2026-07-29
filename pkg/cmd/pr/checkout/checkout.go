package checkout

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	cliContext "github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type CheckoutOptions struct {
	HttpClient func() (*http.Client, error)
	GitClient  *git.Client
	Config     func() (gh.Config, error)
	IO         *iostreams.IOStreams
	Remotes    func() (cliContext.Remotes, error)
	Branch     func() (string, error)

	PRResolver PRResolver

	RecurseSubmodules bool
	Force             bool
	Detach            bool
	BranchName        string
	Worktree          string
}

func NewCmdCheckout(f *cmdutil.Factory, runF func(*CheckoutOptions) error) *cobra.Command {
	opts := &CheckoutOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		GitClient:  f.GitClient,
		Config:     f.Config,
		Remotes:    f.Remotes,
		Branch:     f.Branch,
	}

	cmd := &cobra.Command{
		Use:   "checkout [<number> | <url> | <branch>]",
		Short: "Check out a pull request in git",
		Example: heredoc.Doc(`
			# Interactively select a PR from the 10 most recent to check out
			$ gh pr checkout

			# Checkout a specific PR
			$ gh pr checkout 32
			$ gh pr checkout https://github.com/OWNER/REPO/pull/32
			$ gh pr checkout feature
		`),
		Args:    cobra.MaximumNArgs(1),
		Aliases: []string{"co"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("worktree") && opts.Worktree == "" {
				return cmdutil.FlagErrorf("--worktree cannot be blank")
			}

			if len(args) > 0 {
				opts.PRResolver = &specificPRResolver{
					prFinder: shared.NewFinder(f),
					selector: args[0],
				}
			} else if opts.IO.CanPrompt() {
				baseRepo, err := f.BaseRepo()
				if err != nil {
					return err
				}

				httpClient, err := f.HttpClient()
				if err != nil {
					return err
				}

				opts.PRResolver = &promptingPRResolver{
					io:       opts.IO,
					prompter: f.Prompter,
					prLister: shared.NewLister(httpClient),
					baseRepo: baseRepo,
				}
			} else {
				return cmdutil.FlagErrorf("pull request number, URL, or branch required when not running interactively")
			}

			if runF != nil {
				return runF(opts)
			}
			return checkoutRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.RecurseSubmodules, "recurse-submodules", "", false, "Update all submodules after checkout")
	cmd.Flags().BoolVarP(&opts.Force, "force", "f", false, "Reset the existing local branch to the latest state of the pull request")
	cmd.Flags().BoolVarP(&opts.Detach, "detach", "", false, "Checkout PR with a detached HEAD")
	cmd.Flags().StringVarP(&opts.BranchName, "branch", "b", "", "Local branch name to use (default [the name of the head branch])")
	cmd.Flags().StringVar(&opts.Worktree, "worktree", "", "Check out the pull request into a worktree at the given `path`")

	return cmd
}

func checkoutRun(opts *CheckoutOptions) error {
	pr, baseRepo, err := opts.PRResolver.Resolve()
	if err != nil {
		return err
	}

	var reuseWorktree bool
	if opts.Worktree != "" {
		if err := ensureWorktreePathSafe(opts.Worktree); err != nil {
			return err
		}
		target, err := resolveWorktreeTarget(opts.GitClient, opts.Worktree)
		if err != nil {
			return err
		}
		if target.isCurrentWorktree {
			return fmt.Errorf("--worktree path is the current worktree; omit --worktree to check out here")
		}
		reuseWorktree = target.isExistingWorktree
	}

	cfg, err := opts.Config()
	if err != nil {
		return err
	}
	protocol := cfg.GitProtocol(baseRepo.RepoHost()).Value

	remotes, err := opts.Remotes()
	if err != nil {
		return err
	}
	baseRemote, _ := remotes.FindByRepo(baseRepo.RepoOwner(), baseRepo.RepoName())
	baseURLOrName := ghrepo.FormatRemoteURL(baseRepo, protocol)
	if baseRemote != nil {
		baseURLOrName = baseRemote.Name
	}

	headRemote := baseRemote
	if pr.HeadRepository == nil {
		headRemote = nil
	} else if pr.IsCrossRepository {
		headRemote, _ = remotes.FindByRepo(pr.HeadRepositoryOwner.Login, pr.HeadRepository.Name)
	}

	if strings.HasPrefix(pr.HeadRefName, "-") {
		return fmt.Errorf("invalid branch name: %q", pr.HeadRefName)
	}

	var cmdQueue [][]string

	if headRemote != nil {
		cmdQueue = append(cmdQueue, cmdsForExistingRemote(headRemote, pr, opts, reuseWorktree)...)
	} else {
		httpClient, err := opts.HttpClient()
		if err != nil {
			return err
		}
		apiClient := api.NewClientFromHTTP(httpClient)

		defaultBranch, err := api.RepoDefaultBranch(apiClient, baseRepo)
		if err != nil {
			return err
		}
		cmdQueue = append(cmdQueue, cmdsForMissingRemote(pr, baseURLOrName, baseRepo.RepoHost(), defaultBranch, protocol, opts, reuseWorktree)...)
	}

	if opts.RecurseSubmodules {
		// Run submodule commands inside the worktree when checking out into
		// one, so its submodules (not the main worktree's) get initialized.
		var prefix []string
		if opts.Worktree != "" {
			prefix = []string{"-C", opts.Worktree}
		}
		cmdQueue = append(cmdQueue, slices.Concat(prefix, []string{"submodule", "sync", "--recursive"}))
		cmdQueue = append(cmdQueue, slices.Concat(prefix, []string{"submodule", "update", "--init", "--recursive"}))
	}

	// Note that although we will probably be fetching from the head, in practice, PR checkout can only
	// ever point to one host, and we know baseRepo must be populated.
	err = executeCmds(opts.GitClient, git.CredentialPatternFromHost(baseRepo.RepoHost()), cmdQueue)
	if err != nil {
		return err
	}

	if opts.Worktree != "" && opts.IO.IsStdoutTTY() {
		cs := opts.IO.ColorScheme()
		fmt.Fprintf(opts.IO.ErrOut, "%s Checked out PR #%d in worktree %s\n", cs.SuccessIcon(), pr.Number, opts.Worktree)
		fmt.Fprintf(opts.IO.ErrOut, "  To start working: cd %s\n", opts.Worktree)
	}

	return nil
}

func cmdsForExistingRemote(remote *cliContext.Remote, pr *api.PullRequest, opts *CheckoutOptions, reuseWorktree bool) [][]string {
	var cmds [][]string
	remoteBranch := fmt.Sprintf("%s/%s", remote.Name, pr.HeadRefName)

	refSpec := fmt.Sprintf("+refs/heads/%s", pr.HeadRefName)
	if !opts.Detach {
		refSpec += fmt.Sprintf(":refs/remotes/%s", remoteBranch)
	}

	localBranch := pr.HeadRefName
	if opts.BranchName != "" {
		localBranch = opts.BranchName
	}

	remoteBranchRef := fmt.Sprintf("refs/remotes/%s", remoteBranch)
	fetchCmd := []string{"fetch", remote.Name, refSpec, "--no-tags"}

	if opts.Detach {
		return append(cmds, detachCmds(fetchCmd, opts.Worktree, reuseWorktree)...)
	}

	cmds = append(cmds, fetchCmd)

	switch {
	case opts.Worktree != "":
		if reuseWorktree {
			if localBranchExists(opts.GitClient, localBranch) {
				cmds = append(cmds, worktreeCheckoutCmds(opts.Worktree, localBranch, remoteBranchRef, opts.Force)...)
			} else {
				// New --branch name while reusing a worktree: create it tracking the remote.
				cmds = append(cmds, []string{"-C", opts.Worktree, "checkout", "-b", localBranch, "--track", remoteBranch})
			}
		} else if localBranchExists(opts.GitClient, localBranch) {
			cmds = append(cmds, []string{"worktree", "add", opts.Worktree, localBranch})
			cmds = append(cmds, syncBranchCmds(opts.Worktree, remoteBranchRef, opts.Force)...)
		} else {
			cmds = append(cmds, []string{"worktree", "add", "--track", "-b", localBranch, opts.Worktree, remoteBranch})
		}
	case localBranchExists(opts.GitClient, localBranch):
		cmds = append(cmds, []string{"checkout", localBranch})
		cmds = append(cmds, syncBranchCmds("", remoteBranchRef, opts.Force)...)
	default:
		cmds = append(cmds, []string{"checkout", "-b", localBranch, "--track", remoteBranch})
	}

	return cmds
}

func cmdsForMissingRemote(pr *api.PullRequest, baseURLOrName, repoHost, defaultBranch, protocol string, opts *CheckoutOptions, reuseWorktree bool) [][]string {
	var cmds [][]string
	ref := fmt.Sprintf("refs/pull/%d/head", pr.Number)

	if opts.Detach {
		fetchCmd := []string{"fetch", baseURLOrName, ref, "--no-tags"}
		return detachCmds(fetchCmd, opts.Worktree, reuseWorktree)
	}

	localBranch := pr.HeadRefName
	if opts.BranchName != "" {
		localBranch = opts.BranchName
	} else if pr.HeadRefName == defaultBranch {
		// avoid naming the new branch the same as the default branch
		localBranch = fmt.Sprintf("%s/%s", pr.HeadRepositoryOwner.Login, localBranch)
	}

	currentBranch, _ := opts.Branch()
	if opts.Worktree != "" {
		if reuseWorktree {
			// FETCH_HEAD is per-worktree, and git refuses to update a branch via
			// refspec while it is checked out, so fetch to FETCH_HEAD inside the worktree.
			cmds = append(cmds, []string{"-C", opts.Worktree, "fetch", baseURLOrName, ref, "--no-tags"})
			if localBranchExists(opts.GitClient, localBranch) {
				cmds = append(cmds, []string{"-C", opts.Worktree, "checkout", localBranch})
				cmds = append(cmds, syncBranchCmds(opts.Worktree, "FETCH_HEAD", opts.Force)...)
			} else {
				cmds = append(cmds, []string{"-C", opts.Worktree, "checkout", "-b", localBranch, "FETCH_HEAD"})
			}
		} else {
			fetchCmd := []string{"fetch", baseURLOrName, fmt.Sprintf("%s:%s", ref, localBranch), "--no-tags"}
			if opts.Force {
				fetchCmd = append(fetchCmd, "--force")
			}
			cmds = append(cmds, fetchCmd)
			cmds = append(cmds, []string{"worktree", "add", opts.Worktree, localBranch})
		}
	} else if localBranch == currentBranch {
		// PR head matches currently checked out branch
		cmds = append(cmds, []string{"fetch", baseURLOrName, ref, "--no-tags"})
		cmds = append(cmds, syncBranchCmds("", "FETCH_HEAD", opts.Force)...)
	} else {
		// TODO: check if non-fast-forward and suggest to use `--force`
		fetchCmd := []string{"fetch", baseURLOrName, fmt.Sprintf("%s:%s", ref, localBranch), "--no-tags"}
		if opts.Force {
			fetchCmd = append(fetchCmd, "--force")
		}
		cmds = append(cmds, fetchCmd)
		cmds = append(cmds, []string{"checkout", localBranch})
	}

	remote := baseURLOrName
	mergeRef := ref
	if pr.MaintainerCanModify && pr.HeadRepository != nil {
		headRepo := ghrepo.NewWithHost(pr.HeadRepositoryOwner.Login, pr.HeadRepository.Name, repoHost)
		remote = ghrepo.FormatRemoteURL(headRepo, protocol)
		mergeRef = fmt.Sprintf("refs/heads/%s", pr.HeadRefName)
	}
	if missingMergeConfigForBranch(opts.GitClient, localBranch) {
		// .remote is needed for `git pull` to work
		// .pushRemote is needed for `git push` to work, if user has set `remote.pushDefault`.
		// see https://git-scm.com/docs/git-config#Documentation/git-config.txt-branchltnamegtremote
		cmds = append(cmds, []string{"config", fmt.Sprintf("branch.%s.remote", localBranch), remote})
		cmds = append(cmds, []string{"config", fmt.Sprintf("branch.%s.pushRemote", localBranch), remote})
		cmds = append(cmds, []string{"config", fmt.Sprintf("branch.%s.merge", localBranch), mergeRef})
	}

	return cmds
}

func missingMergeConfigForBranch(client *git.Client, b string) bool {
	mc, err := client.Config(context.Background(), fmt.Sprintf("branch.%s.merge", b))
	return err != nil || mc == ""
}

func localBranchExists(client *git.Client, b string) bool {
	_, err := client.ShowRefs(context.Background(), []string{"refs/heads/" + b})
	return err == nil
}

// worktreeTarget describes a --worktree path, resolved once up front so the
// command builders stay pure instead of each re-querying git.
type worktreeTarget struct {
	// isCurrentWorktree means the path is the worktree we are already running
	// in. Checking out there would silently switch its branch, so callers reject it.
	isCurrentWorktree bool
	// isExistingWorktree means the path is the root of an existing linked
	// worktree of this repo, so we reuse it rather than creating a new one.
	isExistingWorktree bool
}

// resolveWorktreeTarget asks git about path and the current worktree, letting
// git resolve symlinks, "..", case, and trailing slashes for us instead of
// comparing paths ourselves. Detection is best-effort: if either worktree
// cannot be determined (e.g. the path does not exist yet), the flags stay false
// so normal flow proceeds and git worktree add handles the path.
func resolveWorktreeTarget(client *git.Client, path string) (worktreeTarget, error) {
	var wt worktreeTarget
	abs, err := filepath.Abs(path)
	if err != nil {
		return wt, err
	}

	current, err := revParseFacts(client, "", "--show-toplevel", "--git-common-dir")
	if err != nil || len(current) < 2 {
		return wt, nil
	}
	currentToplevel, currentCommonDir := current[0], current[len(current)-1]

	// A non-existent or non-git target errors out here, leaving both flags false.
	target, err := revParseFacts(client, abs, "--show-toplevel", "--show-prefix", "--git-common-dir")
	if err != nil || len(target) < 3 {
		return wt, nil
	}
	targetToplevel, targetPrefix, targetCommonDir := target[0], target[1], target[2]

	wt.isCurrentWorktree = targetToplevel == currentToplevel
	// A worktree root of this repo has an empty prefix and shares our common dir.
	wt.isExistingWorktree = targetPrefix == "" && targetCommonDir == currentCommonDir
	return wt, nil
}

// revParseFacts runs `git rev-parse --path-format=absolute <flags...>` and
// returns one absolute path per flag, in flag order (an empty --show-prefix
// yields an empty string). When dir is non-empty the query is scoped there with -C.
func revParseFacts(client *git.Client, dir string, flags ...string) ([]string, error) {
	args := append([]string{"rev-parse", "--path-format=absolute"}, flags...)
	if dir != "" {
		args = append([]string{"-C", dir}, args...)
	}
	cmd, err := client.Command(context.Background(), args...)
	if err != nil {
		return nil, err
	}
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return strings.Split(strings.TrimRight(string(out), "\n"), "\n"), nil
}

// detachCmds returns the commands for a detached checkout. When reusing an
// existing linked worktree, FETCH_HEAD must be written inside it (it is
// per-worktree), so the fetch runs with -C <path>.
func detachCmds(fetchCmd []string, worktree string, reuseWorktree bool) [][]string {
	if worktree == "" {
		return [][]string{
			fetchCmd,
			{"checkout", "--detach", "FETCH_HEAD"},
		}
	}

	if reuseWorktree {
		return [][]string{
			append([]string{"-C", worktree}, fetchCmd...),
			{"-C", worktree, "checkout", "--detach", "FETCH_HEAD"},
		}
	}
	return [][]string{
		fetchCmd,
		{"worktree", "add", "--detach", worktree, "FETCH_HEAD"},
	}
}

// syncBranchCmds syncs a branch to ref: a hard reset when force is set,
// otherwise a fast-forward-only merge. A non-empty path runs the commands there.
func syncBranchCmds(path, ref string, force bool) [][]string {
	var prefix []string
	if path != "" {
		prefix = []string{"-C", path}
	}
	if force {
		return [][]string{append(prefix, "reset", "--hard", ref)}
	}
	// TODO: check if non-fast-forward and suggest to use `--force`
	return [][]string{append(prefix, "merge", "--ff-only", ref)}
}

func worktreeCheckoutCmds(path, branch, ref string, force bool) [][]string {
	cmds := [][]string{{"-C", path, "checkout", branch}}
	cmds = append(cmds, syncBranchCmds(path, ref, force)...)
	return cmds
}

// ensureWorktreePathSafe validates a --worktree target before we write to it:
// it must be a non-existent path (git will create it) or an existing directory,
// and never a symlink at its final component. A symlinked ancestor (e.g. macOS
// /tmp -> /private/tmp) is fine; os.Lstat checks only the leaf so it is not
// followed. Rejecting a leaf symlink guards against writing PR content through a
// planted link.
func ensureWorktreePathSafe(path string) error {
	fi, err := os.Lstat(path)
	switch {
	case os.IsNotExist(err):
		return nil
	case err != nil:
		return err
	case fi.Mode()&os.ModeSymlink != 0:
		return fmt.Errorf("--worktree path must not be a symlink: %s", path)
	case !fi.IsDir():
		return fmt.Errorf("--worktree path must be a directory: %s", path)
	}
	return nil
}

func executeCmds(client *git.Client, credentialPattern git.CredentialPattern, cmdQueue [][]string) error {
	for _, args := range cmdQueue {
		// Determine the git sub-command, skipping any -C <path> prefix.
		subCmd := args[0]
		if len(args) >= 3 && args[0] == "-C" {
			subCmd = args[2]
		}

		var err error
		var cmd *git.Command
		switch subCmd {
		case "submodule":
			cmd, err = authenticatedCommand(client, credentialPattern, args)
		case "fetch":
			cmd, err = authenticatedCommand(client, git.AllMatchingCredentialsPattern, args)
		default:
			cmd, err = client.Command(context.Background(), args...)
		}
		if err != nil {
			return err
		}
		if err := cmd.Run(); err != nil {
			return err
		}
	}
	return nil
}

// authenticatedCommand builds an authenticated git command, transparently
// handling a leading -C <path> prefix. AuthenticatedCommand prepends
// credential-helper flags before all args, so a -C prefix would be displaced;
// instead we strip it and apply it as cmd.Dir.
func authenticatedCommand(client *git.Client, credentialPattern git.CredentialPattern, args []string) (*git.Command, error) {
	if args[0] == "-C" {
		cmd, err := client.AuthenticatedCommand(context.Background(), credentialPattern, args[2:]...)
		if err != nil {
			return nil, err
		}
		cmd.Dir = args[1]
		return cmd, nil
	}
	return client.AuthenticatedCommand(context.Background(), credentialPattern, args...)
}

type PRResolver interface {
	Resolve() (*api.PullRequest, ghrepo.Interface, error)
}

type specificPRResolver struct {
	prFinder shared.PRFinder
	selector string
}

func (r *specificPRResolver) Resolve() (*api.PullRequest, ghrepo.Interface, error) {
	pr, baseRepo, err := r.prFinder.Find(shared.FindOptions{
		Selector: r.selector,
		Fields: []string{
			"number",
			"headRefName",
			"headRepository",
			"headRepositoryOwner",
			"isCrossRepository",
			"maintainerCanModify",
		},
	})
	if err != nil {
		return nil, nil, err
	}
	return pr, baseRepo, nil
}

type promptingPRResolver struct {
	io       *iostreams.IOStreams
	prompter shared.Prompt

	prLister shared.PRLister

	baseRepo ghrepo.Interface
}

func (r *promptingPRResolver) Resolve() (*api.PullRequest, ghrepo.Interface, error) {
	r.io.StartProgressIndicator()
	listResult, err := r.prLister.List(shared.ListOptions{
		BaseRepo: r.baseRepo,
		State:    "open",
		Fields: []string{
			"number",
			"title",
			"state",
			"isDraft",

			"headRefName",
			"headRepository",
			"headRepositoryOwner",
			"isCrossRepository",
			"maintainerCanModify",
		},
		LimitResults: 10})
	r.io.StopProgressIndicator()
	if err != nil {
		return nil, nil, err
	}
	if len(listResult.PullRequests) == 0 {
		return nil, nil, shared.ListNoResults(ghrepo.FullName(r.baseRepo), "pull request", false)
	}

	candidates := []string{}
	for _, pr := range listResult.PullRequests {
		candidates = append(candidates, fmt.Sprintf("%d\t%s %s [%s]",
			pr.Number,
			shared.PrStateWithDraft(&pr),
			text.RemoveExcessiveWhitespace(pr.Title),
			pr.HeadLabel(),
		))
	}

	selected, err := r.prompter.Select("Select a pull request", "", candidates)
	if err != nil {
		return nil, nil, err
	}

	return &listResult.PullRequests[selected], r.baseRepo, nil
}
