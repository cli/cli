package checkout

import (
	"context"
	"fmt"
	"net/http"
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
			$ gh pr checkout 32 --branch feature --worktree /path/to/wt-feature
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
		target, err := shared.ResolveWorktreeTarget(opts.GitClient, opts.Worktree)
		if err != nil {
			return err
		}
		reuseWorktree = target.Reuse
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
		worktreeCmds, branchExists := shared.WorktreeCheckoutCommands(
			opts.GitClient,
			shared.WorktreeTarget{Path: opts.Worktree, Reuse: reuseWorktree},
			localBranch,
			remoteBranch,
		)
		cmds = append(cmds, worktreeCmds...)
		if branchExists {
			cmds = append(cmds, syncBranchCmds(opts.Worktree, remoteBranchRef, opts.Force)...)
		}
	case opts.GitClient.HasLocalBranch(context.Background(), localBranch):
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
			if opts.GitClient.HasLocalBranch(context.Background(), localBranch) {
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
			cmds = append(cmds, []string{"worktree", "add", "--", opts.Worktree, localBranch})
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
		{"worktree", "add", "--detach", "--", worktree, "FETCH_HEAD"},
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
