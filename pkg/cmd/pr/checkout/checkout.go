package checkout

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
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
		cmdQueue = append(cmdQueue, cmdsForExistingRemote(headRemote, pr, opts)...)
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
		cmdQueue = append(cmdQueue, cmdsForMissingRemote(pr, baseURLOrName, baseRepo.RepoHost(), defaultBranch, protocol, opts)...)
	}

	if opts.RecurseSubmodules {
		cmdQueue = append(cmdQueue, []string{"submodule", "sync", "--recursive"})
		cmdQueue = append(cmdQueue, []string{"submodule", "update", "--init", "--recursive"})
	}

	// Note that although we will probably be fetching from the head, in practice, PR checkout can only
	// ever point to one host, and we know baseRepo must be populated.
	err = executeCmds(opts.GitClient, git.CredentialPatternFromHost(baseRepo.RepoHost()), cmdQueue)
	if err != nil {
		return err
	}

	if opts.Worktree != "" && opts.IO.IsStdoutTTY() {
		cs := opts.IO.ColorScheme()
		fmt.Fprintf(opts.IO.Out, "%s Worktree ready for PR #%d\n", cs.SuccessIcon(), pr.Number)
		fmt.Fprintf(opts.IO.Out, "  %s\n", opts.Worktree)
		fmt.Fprintf(opts.IO.Out, "  To start working: cd %q\n", opts.Worktree)
	}

	return nil
}

func cmdsForExistingRemote(remote *cliContext.Remote, pr *api.PullRequest, opts *CheckoutOptions) [][]string {
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

	// FETCH_HEAD is per-worktree: when reusing an existing linked worktree in
	// detach mode, fetch inside it so FETCH_HEAD is written there.
	if opts.Detach && opts.Worktree != "" && isWorktreeAtPath(opts.GitClient, opts.Worktree) {
		cmds = append(cmds, append([]string{"-C", opts.Worktree}, fetchCmd...))
		cmds = append(cmds, []string{"-C", opts.Worktree, "checkout", "--detach", "FETCH_HEAD"})
		return cmds
	}

	cmds = append(cmds, fetchCmd)

	switch {
	case opts.Detach:
		if opts.Worktree != "" {
			cmds = append(cmds, []string{"worktree", "add", "--detach", opts.Worktree, "FETCH_HEAD"})
		} else {
			cmds = append(cmds, []string{"checkout", "--detach", "FETCH_HEAD"})
		}
	case opts.Worktree != "":
		if isWorktreeAtPath(opts.GitClient, opts.Worktree) {
			cmds = append(cmds, worktreeCheckoutCmds(opts.Worktree, localBranch, remoteBranchRef, opts.Force)...)
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

func cmdsForMissingRemote(pr *api.PullRequest, baseURLOrName, repoHost, defaultBranch, protocol string, opts *CheckoutOptions) [][]string {
	var cmds [][]string
	ref := fmt.Sprintf("refs/pull/%d/head", pr.Number)

	if opts.Detach {
		fetchCmd := []string{"fetch", baseURLOrName, ref, "--no-tags"}
		if opts.Worktree != "" && isWorktreeAtPath(opts.GitClient, opts.Worktree) {
			// FETCH_HEAD is per-worktree; fetch inside the linked worktree.
			cmds = append(cmds, append([]string{"-C", opts.Worktree}, fetchCmd...))
			cmds = append(cmds, []string{"-C", opts.Worktree, "checkout", "--detach", "FETCH_HEAD"})
		} else {
			cmds = append(cmds, fetchCmd)
			if opts.Worktree != "" {
				cmds = append(cmds, []string{"worktree", "add", "--detach", opts.Worktree, "FETCH_HEAD"})
			} else {
				cmds = append(cmds, []string{"checkout", "--detach", "FETCH_HEAD"})
			}
		}
		return cmds
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
		if isWorktreeAtPath(opts.GitClient, opts.Worktree) {
			// FETCH_HEAD is per-worktree; fetch inside the linked worktree
			// rather than the main worktree. We fetch to FETCH_HEAD because
			// git refuses to update a branch via refspec when it is checked
			// out in a worktree.
			cmds = append(cmds, []string{"-C", opts.Worktree, "fetch", baseURLOrName, ref, "--no-tags"})
			// Use checkout -B to create-or-reset the branch from FETCH_HEAD.
			// The local branch may not exist yet (e.g. switching the worktree
			// to a different fork PR).
			cmds = append(cmds, []string{"-C", opts.Worktree, "checkout", "-B", localBranch, "FETCH_HEAD"})
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

// isWorktreeAtPath reports whether the given path is a registered git worktree.
func isWorktreeAtPath(client *git.Client, path string) bool {
	cmd, err := client.Command(context.Background(), "worktree", "list", "--porcelain")
	if err != nil {
		return false
	}
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	resolved := resolvePath(path)
	for _, line := range strings.Split(string(out), "\n") {
		if p, ok := strings.CutPrefix(line, "worktree "); ok {
			if resolvePath(p) == resolved {
				return true
			}
		}
	}
	return false
}

// syncBranchCmds returns commands that sync a branch to ref: a hard reset when
// force is set, otherwise a fast-forward-only merge. If path is non-empty, the
// commands are prefixed with -C to run inside that directory.
func syncBranchCmds(path, ref string, force bool) [][]string {
	var prefix []string
	if path != "" {
		prefix = []string{"-C", path}
	}
	if force {
		return [][]string{append(prefix, "reset", "--hard", ref)}
	}
	return [][]string{append(prefix, "merge", "--ff-only", ref)}
}

// worktreeCheckoutCmds returns commands to switch an existing worktree to the
// given branch and sync it. Git will refuse if there are conflicting local changes.
func worktreeCheckoutCmds(path, branch, ref string, force bool) [][]string {
	cmds := [][]string{{"-C", path, "checkout", branch}}
	cmds = append(cmds, syncBranchCmds(path, ref, force)...)
	return cmds
}

// resolvePath canonicalizes a path for comparison against git-reported worktree
// paths. Git resolves symlinks internally, so on systems where common directories
// are symlinks (e.g. macOS /tmp -> /private/tmp), the user-provided path and the
// path git reports would otherwise not match.
func resolvePath(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved
	}
	return p
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
			cmd, err = client.AuthenticatedCommand(context.Background(), credentialPattern, args...)
		case "fetch":
			// AuthenticatedCommand prepends credential-helper flags
			// before all args. When -C <path> is present, strip it and
			// apply as cmd.Dir so the flags don't displace it.
			if args[0] == "-C" {
				cmd, err = client.AuthenticatedCommand(context.Background(), git.AllMatchingCredentialsPattern, args[2:]...)
				if err == nil {
					cmd.Dir = args[1]
				}
			} else {
				cmd, err = client.AuthenticatedCommand(context.Background(), git.AllMatchingCredentialsPattern, args...)
			}
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
