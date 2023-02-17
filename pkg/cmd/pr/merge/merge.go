package merge

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	ghContext "github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/surveyext"
	"github.com/spf13/cobra"
)

type editor interface {
	Edit(string, string) (string, error)
}

type MergeOptions struct {
	HttpClient func() (*http.Client, error)
	GitClient  *git.Client
	IO         *iostreams.IOStreams
	Branch     func() (string, error)
	Remotes    func() (ghContext.Remotes, error)
	Prompter   shared.Prompt

	Finder shared.PRFinder

	SelectorArg  string
	DeleteBranch bool
	MergeMethod  PullRequestMergeMethod

	AutoMergeEnable  bool
	AutoMergeDisable bool

	AuthorEmail string

	Body    string
	BodySet bool
	Subject string
	Editor  editor

	UseAdmin                bool
	IsDeleteBranchIndicated bool
	CanDeleteLocalBranch    bool
	MergeStrategyEmpty      bool
	MatchHeadCommit         string
}

// ErrAlreadyInMergeQueue indicates that the pull request is already in a merge queue
var ErrAlreadyInMergeQueue = errors.New("already in merge queue")

func NewCmdMerge(f *cmdutil.Factory, runF func(*MergeOptions) error) *cobra.Command {
	opts := &MergeOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		GitClient:  f.GitClient,
		Branch:     f.Branch,
		Remotes:    f.Remotes,
		Prompter:   f.Prompter,
	}

	var (
		flagMerge  bool
		flagSquash bool
		flagRebase bool
	)

	var bodyFile string

	cmd := &cobra.Command{
		Use:   "merge [<number> | <url> | <branch>]",
		Short: "Merge a pull request",
		Long: heredoc.Doc(`
			Merge a pull request on GitHub.

			Without an argument, the pull request that belongs to the current branch
			is selected.

			When targeting a branch that requires a merge queue, no merge strategy is required.
			If required checks have not yet passed, AutoMerge will be enabled.
			If required checks have passed, the pull request will be added to the merge queue.
			To bypass a merge queue and merge directly, pass the '--admin' flag.
    	`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Finder = shared.NewFinder(f)

			if repoOverride, _ := cmd.Flags().GetString("repo"); repoOverride != "" && len(args) == 0 {
				return cmdutil.FlagErrorf("argument required when using the --repo flag")
			}

			if len(args) > 0 {
				opts.SelectorArg = args[0]
			}

			methodFlags := 0
			if flagMerge {
				opts.MergeMethod = PullRequestMergeMethodMerge
				methodFlags++
			}
			if flagRebase {
				opts.MergeMethod = PullRequestMergeMethodRebase
				methodFlags++
			}
			if flagSquash {
				opts.MergeMethod = PullRequestMergeMethodSquash
				methodFlags++
			}
			if methodFlags == 0 {
				opts.MergeStrategyEmpty = true
			} else if methodFlags > 1 {
				return cmdutil.FlagErrorf("only one of --merge, --rebase, or --squash can be enabled")
			}

			opts.IsDeleteBranchIndicated = cmd.Flags().Changed("delete-branch")
			opts.CanDeleteLocalBranch = !cmd.Flags().Changed("repo")

			bodyProvided := cmd.Flags().Changed("body")
			bodyFileProvided := bodyFile != ""

			if err := cmdutil.MutuallyExclusive(
				"specify only one of `--auto`, `--disable-auto`, or `--admin`",
				opts.AutoMergeEnable,
				opts.AutoMergeDisable,
				opts.UseAdmin,
			); err != nil {
				return err
			}

			if err := cmdutil.MutuallyExclusive(
				"specify only one of `--body` or `--body-file`",
				bodyProvided,
				bodyFileProvided,
			); err != nil {
				return err
			}

			if bodyProvided || bodyFileProvided {
				opts.BodySet = true
				if bodyFileProvided {
					b, err := cmdutil.ReadFile(bodyFile, opts.IO.In)
					if err != nil {
						return err
					}
					opts.Body = string(b)
				}
			}

			opts.Editor = &userEditor{
				io:     opts.IO,
				config: f.Config,
			}

			if runF != nil {
				return runF(opts)
			}

			err := mergeRun(opts)
			if errors.Is(err, ErrAlreadyInMergeQueue) {
				return nil
			}
			return err
		},
	}

	cmd.Flags().BoolVar(&opts.UseAdmin, "admin", false, "Use administrator privileges to merge a pull request that does not meet requirements")
	cmd.Flags().BoolVarP(&opts.DeleteBranch, "delete-branch", "d", false, "Delete the local and remote branch after merge")
	cmd.Flags().StringVarP(&opts.Body, "body", "b", "", "Body `text` for the merge commit")
	cmd.Flags().StringVarP(&bodyFile, "body-file", "F", "", "Read body text from `file` (use \"-\" to read from standard input)")
	cmd.Flags().StringVarP(&opts.Subject, "subject", "t", "", "Subject `text` for the merge commit")
	cmd.Flags().BoolVarP(&flagMerge, "merge", "m", false, "Merge the commits with the base branch")
	cmd.Flags().BoolVarP(&flagRebase, "rebase", "r", false, "Rebase the commits onto the base branch")
	cmd.Flags().BoolVarP(&flagSquash, "squash", "s", false, "Squash the commits into one commit and merge it into the base branch")
	cmd.Flags().BoolVar(&opts.AutoMergeEnable, "auto", false, "Automatically merge only after necessary requirements are met")
	cmd.Flags().BoolVar(&opts.AutoMergeDisable, "disable-auto", false, "Disable auto-merge for this pull request")
	cmd.Flags().StringVar(&opts.MatchHeadCommit, "match-head-commit", "", "Commit `SHA` that the pull request head must match to allow merge")
	cmd.Flags().StringVarP(&opts.AuthorEmail, "author-email", "A", "", "Email `text` for merge commit author")
	return cmd
}

// mergeContext contains state and dependencies to merge a pull request.
type mergeContext struct {
	pr                 *api.PullRequest
	baseRepo           ghrepo.Interface
	httpClient         *http.Client
	opts               *MergeOptions
	cs                 *iostreams.ColorScheme
	isTerminal         bool
	merged             bool
	localBranchExists  bool
	autoMerge          bool
	crossRepoPR        bool
	deleteBranch       bool
	switchedToBranch   string
	mergeQueueRequired bool
}

// Attempt to disable auto merge on the pull request.
func (m *mergeContext) disableAutoMerge() error {
	if err := disableAutoMerge(m.httpClient, m.baseRepo, m.pr.ID); err != nil {
		return err
	}
	return m.infof("%s Auto-merge disabled for pull request #%d\n", m.cs.SuccessIconWithColor(m.cs.Green), m.pr.Number)
}

// Check if this pull request is in a merge queue
func (m *mergeContext) inMergeQueue() error {
	// if the pull request is in a merge queue no further action is possible
	if m.pr.IsInMergeQueue {
		_ = m.warnf("%s Pull request #%d is already queued to merge\n", m.cs.WarningIcon(), m.pr.Number)
		return ErrAlreadyInMergeQueue
	}
	return nil
}

// Warn if the pull request and the remote branch have diverged.
func (m *mergeContext) warnIfDiverged() {
	if m.opts.SelectorArg != "" || len(m.pr.Commits.Nodes) == 0 {
		return
	}

	localBranchLastCommit, err := m.opts.GitClient.LastCommit(context.Background())
	if err != nil {
		return
	}

	if localBranchLastCommit.Sha == m.pr.Commits.Nodes[len(m.pr.Commits.Nodes)-1].Commit.OID {
		return
	}

	_ = m.warnf("%s Pull request #%d (%s) has diverged from local branch\n", m.cs.Yellow("!"), m.pr.Number, m.pr.Title)
}

// Check if the current state of the pull request allows for merging
func (m *mergeContext) canMerge() error {
	if m.mergeQueueRequired {
		// a pull request can always be added to the merge queue
		return nil
	}

	reason := blockedReason(m.pr.MergeStateStatus, m.opts.UseAdmin)

	if reason == "" || m.autoMerge || m.merged {
		return nil
	}

	_ = m.warnf("%s Pull request #%d is not mergeable: %s.\n", m.cs.FailureIcon(), m.pr.Number, reason)
	_ = m.warnf("To have the pull request merged after all the requirements have been met, add the `--auto` flag.\n")
	if remote := remoteForMergeConflictResolution(m.baseRepo, m.pr, m.opts); remote != nil {
		mergeOrRebase := "merge"
		if m.opts.MergeMethod == PullRequestMergeMethodRebase {
			mergeOrRebase = "rebase"
		}
		fetchBranch := fmt.Sprintf("%s %s", remote.Name, m.pr.BaseRefName)
		mergeBranch := fmt.Sprintf("%s %s/%s", mergeOrRebase, remote.Name, m.pr.BaseRefName)
		cmd := fmt.Sprintf("gh pr checkout %d && git fetch %s && git %s", m.pr.Number, fetchBranch, mergeBranch)
		_ = m.warnf("Run the following to resolve the merge conflicts locally:\n  %s\n", m.cs.Bold(cmd))
	}
	if !m.opts.UseAdmin && allowsAdminOverride(m.pr.MergeStateStatus) {
		// TODO: show this flag only to repo admins
		_ = m.warnf("To use administrator privileges to immediately merge the pull request, add the `--admin` flag.\n")
	}
	return cmdutil.SilentError
}

// Merge the pull request. May prompt the user for input parameters for the merge.
func (m *mergeContext) merge() error {
	if m.merged {
		return nil
	}

	payload := mergePayload{
		repo:            m.baseRepo,
		pullRequestID:   m.pr.ID,
		method:          m.opts.MergeMethod,
		auto:            m.autoMerge,
		commitSubject:   m.opts.Subject,
		commitBody:      m.opts.Body,
		setCommitBody:   m.opts.BodySet,
		expectedHeadOid: m.opts.MatchHeadCommit,
		authorEmail:     m.opts.AuthorEmail,
	}

	if m.shouldAddToMergeQueue() {
		if !m.opts.MergeStrategyEmpty {
			// only warn for now
			_ = m.warnf("%s The merge strategy for %s is set by the merge queue\n", m.cs.Yellow("!"), m.pr.BaseRefName)
		}
		// auto merge will either enable auto merge or add to the merge queue
		payload.auto = true
	} else {
		// get user input if not already given
		if m.opts.MergeStrategyEmpty {
			if !m.opts.IO.CanPrompt() {
				return cmdutil.FlagErrorf("--merge, --rebase, or --squash required when not running interactively")
			}

			apiClient := api.NewClientFromHTTP(m.httpClient)
			r, err := api.GitHubRepo(apiClient, m.baseRepo)
			if err != nil {
				return err
			}

			payload.method, err = mergeMethodSurvey(m.opts.Prompter, r)
			if err != nil {
				return err
			}

			m.deleteBranch, err = deleteBranchSurvey(m.opts, m.crossRepoPR, m.localBranchExists)
			if err != nil {
				return err
			}

			allowEditMsg := payload.method != PullRequestMergeMethodRebase
			for {
				action, err := confirmSurvey(m.opts.Prompter, allowEditMsg)
				if err != nil {
					return fmt.Errorf("unable to confirm: %w", err)
				}

				submit, err := confirmSubmission(m.httpClient, m.opts, action, &payload)
				if err != nil {
					return err
				}
				if submit {
					break
				}
			}
		}
	}

	err := mergePullRequest(m.httpClient, payload)
	if err != nil {
		return err
	}

	if m.shouldAddToMergeQueue() {
		_ = m.infof("%s Pull request #%d will be added to the merge queue for %s when ready\n", m.cs.SuccessIconWithColor(m.cs.Green), m.pr.Number, m.pr.BaseRefName)
		return nil
	}

	if payload.auto {
		method := ""
		switch payload.method {
		case PullRequestMergeMethodRebase:
			method = " via rebase"
		case PullRequestMergeMethodSquash:
			method = " via squash"
		}
		return m.infof("%s Pull request #%d will be automatically merged%s when all requirements are met\n", m.cs.SuccessIconWithColor(m.cs.Green), m.pr.Number, method)
	}

	action := "Merged"
	switch payload.method {
	case PullRequestMergeMethodRebase:
		action = "Rebased and merged"
	case PullRequestMergeMethodSquash:
		action = "Squashed and merged"
	}
	return m.infof("%s %s pull request #%d (%s)\n", m.cs.SuccessIconWithColor(m.cs.Magenta), action, m.pr.Number, m.pr.Title)
}

// Delete local branch if requested and if allowed.
func (m *mergeContext) deleteLocalBranch() error {
	if m.crossRepoPR || m.autoMerge {
		return nil
	}

	if m.merged {
		if m.opts.IO.CanPrompt() && !m.opts.IsDeleteBranchIndicated {
			confirmed, err := m.opts.Prompter.Confirm(fmt.Sprintf("Pull request #%d was already merged. Delete the branch locally?", m.pr.Number), false)
			if err != nil {
				return fmt.Errorf("could not prompt: %w", err)
			}
			m.deleteBranch = confirmed
		} else {
			_ = m.warnf(fmt.Sprintf("%s Pull request #%d was already merged\n", m.cs.WarningIcon(), m.pr.Number))
		}
	}

	if !m.deleteBranch || !m.opts.CanDeleteLocalBranch || !m.localBranchExists {
		return nil
	}

	currentBranch, err := m.opts.Branch()
	if err != nil {
		return err
	}

	ctx := context.Background()

	// branch the command was run on is the same as the pull request branch
	if currentBranch == m.pr.HeadRefName {
		remotes, err := m.opts.Remotes()
		if err != nil {
			return err
		}

		baseRemote, err := remotes.FindByRepo(m.baseRepo.RepoOwner(), m.baseRepo.RepoName())
		if err != nil {
			return err
		}

		targetBranch := m.pr.BaseRefName
		if m.opts.GitClient.HasLocalBranch(ctx, targetBranch) {
			if err := m.opts.GitClient.CheckoutBranch(ctx, targetBranch); err != nil {
				return err
			}
		} else {
			if err := m.opts.GitClient.CheckoutNewBranch(ctx, baseRemote.Name, targetBranch); err != nil {
				return err
			}
		}

		if err := m.opts.GitClient.Pull(ctx, baseRemote.Name, targetBranch); err != nil {
			_ = m.warnf(fmt.Sprintf("%s warning: not possible to fast-forward to: %q\n", m.cs.WarningIcon(), targetBranch))
		}

		m.switchedToBranch = targetBranch
	}

	if err := m.opts.GitClient.DeleteLocalBranch(ctx, m.pr.HeadRefName); err != nil {
		return fmt.Errorf("failed to delete local branch %s: %w", m.cs.Cyan(m.pr.HeadRefName), err)
	}

	return nil
}

// Delete the remote branch if requested and if allowed.
func (m *mergeContext) deleteRemoteBranch() error {
	// the user was already asked if they want to delete the branch if they didn't provide the flag
	if !m.deleteBranch || m.crossRepoPR || m.autoMerge {
		return nil
	}

	if !m.merged {
		apiClient := api.NewClientFromHTTP(m.httpClient)
		err := api.BranchDeleteRemote(apiClient, m.baseRepo, m.pr.HeadRefName)
		var httpErr api.HTTPError
		// The ref might have already been deleted by GitHub
		if err != nil && (!errors.As(err, &httpErr) || httpErr.StatusCode != 422) {
			return fmt.Errorf("failed to delete remote branch %s: %w", m.cs.Cyan(m.pr.HeadRefName), err)
		}
	}

	branch := ""
	if m.switchedToBranch != "" {
		branch = fmt.Sprintf(" and switched to branch %s", m.cs.Cyan(m.switchedToBranch))
	}
	return m.infof("%s Deleted branch %s%s\n", m.cs.SuccessIconWithColor(m.cs.Red), m.cs.Cyan(m.pr.HeadRefName), branch)
}

// Add the Pull Request to a merge queue
// Admins can bypass the queue and merge directly
func (m *mergeContext) shouldAddToMergeQueue() bool {
	return m.mergeQueueRequired && !m.opts.UseAdmin
}

func (m *mergeContext) warnf(format string, args ...interface{}) error {
	_, err := fmt.Fprintf(m.opts.IO.ErrOut, format, args...)
	return err
}

func (m *mergeContext) infof(format string, args ...interface{}) error {
	if !m.isTerminal {
		return nil
	}
	_, err := fmt.Fprintf(m.opts.IO.ErrOut, format, args...)
	return err
}

// Creates a new MergeConext from MergeOptions.
func NewMergeContext(opts *MergeOptions) (*mergeContext, error) {
	findOptions := shared.FindOptions{
		Selector: opts.SelectorArg,
		Fields:   []string{"id", "number", "state", "title", "lastCommit", "mergeStateStatus", "headRepositoryOwner", "headRefName", "baseRefName", "headRefOid", "isInMergeQueue", "isMergeQueueEnabled"},
	}
	pr, baseRepo, err := opts.Finder.Find(findOptions)
	if err != nil {
		return nil, err
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return nil, err
	}

	return &mergeContext{
		opts:               opts,
		pr:                 pr,
		cs:                 opts.IO.ColorScheme(),
		baseRepo:           baseRepo,
		isTerminal:         opts.IO.IsStdoutTTY(),
		httpClient:         httpClient,
		merged:             pr.State == MergeStateStatusMerged,
		deleteBranch:       opts.DeleteBranch,
		crossRepoPR:        pr.HeadRepositoryOwner.Login != baseRepo.RepoOwner(),
		autoMerge:          opts.AutoMergeEnable && !isImmediatelyMergeable(pr.MergeStateStatus),
		localBranchExists:  opts.CanDeleteLocalBranch && opts.GitClient.HasLocalBranch(context.Background(), pr.HeadRefName),
		mergeQueueRequired: pr.IsMergeQueueEnabled,
	}, nil
}

// Run the merge command.
func mergeRun(opts *MergeOptions) error {
	ctx, err := NewMergeContext(opts)
	if err != nil {
		return err
	}

	if err := ctx.inMergeQueue(); err != nil {
		return err
	}

	// no further action is possible when disabling auto merge
	if opts.AutoMergeDisable {
		return ctx.disableAutoMerge()
	}

	ctx.warnIfDiverged()

	if err := ctx.canMerge(); err != nil {
		return err
	}

	if err := ctx.merge(); err != nil {
		return err
	}

	if err := ctx.deleteLocalBranch(); err != nil {
		return err
	}

	if err := ctx.deleteRemoteBranch(); err != nil {
		return err
	}

	return nil
}

func mergeMethodSurvey(p shared.Prompt, baseRepo *api.Repository) (PullRequestMergeMethod, error) {
	type mergeOption struct {
		title  string
		method PullRequestMergeMethod
	}

	var mergeOpts []mergeOption
	if baseRepo.MergeCommitAllowed {
		opt := mergeOption{title: "Create a merge commit", method: PullRequestMergeMethodMerge}
		mergeOpts = append(mergeOpts, opt)
	}
	if baseRepo.RebaseMergeAllowed {
		opt := mergeOption{title: "Rebase and merge", method: PullRequestMergeMethodRebase}
		mergeOpts = append(mergeOpts, opt)
	}
	if baseRepo.SquashMergeAllowed {
		opt := mergeOption{title: "Squash and merge", method: PullRequestMergeMethodSquash}
		mergeOpts = append(mergeOpts, opt)
	}

	var surveyOpts []string
	for _, v := range mergeOpts {
		surveyOpts = append(surveyOpts, v.title)
	}

	result, err := p.Select("What merge method would you like to use?", "", surveyOpts)

	return mergeOpts[result].method, err
}

func deleteBranchSurvey(opts *MergeOptions, crossRepoPR, localBranchExists bool) (bool, error) {
	if !crossRepoPR && !opts.IsDeleteBranchIndicated {
		var message string
		if opts.CanDeleteLocalBranch && localBranchExists {
			message = "Delete the branch locally and on GitHub?"
		} else {
			message = "Delete the branch on GitHub?"
		}

		return opts.Prompter.Confirm(message, false)
	}

	return opts.DeleteBranch, nil
}

func confirmSurvey(p shared.Prompt, allowEditMsg bool) (shared.Action, error) {
	const (
		submitLabel            = "Submit"
		editCommitSubjectLabel = "Edit commit subject"
		editCommitMsgLabel     = "Edit commit message"
		cancelLabel            = "Cancel"
	)

	options := []string{submitLabel}
	if allowEditMsg {
		options = append(options, editCommitSubjectLabel, editCommitMsgLabel)
	}
	options = append(options, cancelLabel)

	selected, err := p.Select("What's next?", "", options)
	if err != nil {
		return shared.CancelAction, fmt.Errorf("could not prompt: %w", err)
	}

	switch options[selected] {
	case submitLabel:
		return shared.SubmitAction, nil
	case editCommitSubjectLabel:
		return shared.EditCommitSubjectAction, nil
	case editCommitMsgLabel:
		return shared.EditCommitMessageAction, nil
	default:
		return shared.CancelAction, nil
	}
}

func confirmSubmission(client *http.Client, opts *MergeOptions, action shared.Action, payload *mergePayload) (bool, error) {
	var err error

	switch action {
	case shared.EditCommitMessageAction:
		if !payload.setCommitBody {
			_, payload.commitBody, err = getMergeText(client, payload.repo, payload.pullRequestID, payload.method)
			if err != nil {
				return false, err
			}
		}

		payload.commitBody, err = opts.Editor.Edit("*.md", payload.commitBody)
		if err != nil {
			return false, err
		}
		payload.setCommitBody = true

		return false, nil

	case shared.EditCommitSubjectAction:
		if payload.commitSubject == "" {
			payload.commitSubject, _, err = getMergeText(client, payload.repo, payload.pullRequestID, payload.method)
			if err != nil {
				return false, err
			}
		}

		payload.commitSubject, err = opts.Editor.Edit("*.md", payload.commitSubject)
		if err != nil {
			return false, err
		}

		return false, nil

	case shared.CancelAction:
		fmt.Fprintln(opts.IO.ErrOut, "Cancelled.")
		return false, cmdutil.CancelError

	case shared.SubmitAction:
		return true, nil

	default:
		return false, fmt.Errorf("unable to confirm: %w", err)
	}
}

type userEditor struct {
	io     *iostreams.IOStreams
	config func() (config.Config, error)
}

func (e *userEditor) Edit(filename, startingText string) (string, error) {
	editorCommand, err := cmdutil.DetermineEditor(e.config)
	if err != nil {
		return "", err
	}

	return surveyext.Edit(editorCommand, filename, startingText, e.io.In, e.io.Out, e.io.ErrOut)
}

// blockedReason translates various MergeStateStatus GraphQL values into human-readable reason
func blockedReason(status string, useAdmin bool) string {
	switch status {
	case MergeStateStatusBlocked:
		if useAdmin {
			return ""
		}
		return "the base branch policy prohibits the merge"
	case MergeStateStatusBehind:
		if useAdmin {
			return ""
		}
		return "the head branch is not up to date with the base branch"
	case MergeStateStatusDirty:
		return "the merge commit cannot be cleanly created"
	default:
		return ""
	}
}

func allowsAdminOverride(status string) bool {
	switch status {
	case MergeStateStatusBlocked, MergeStateStatusBehind:
		return true
	default:
		return false
	}
}

func remoteForMergeConflictResolution(baseRepo ghrepo.Interface, pr *api.PullRequest, opts *MergeOptions) *ghContext.Remote {
	if !mergeConflictStatus(pr.MergeStateStatus) || !opts.CanDeleteLocalBranch {
		return nil
	}
	remotes, err := opts.Remotes()
	if err != nil {
		return nil
	}
	remote, err := remotes.FindByRepo(baseRepo.RepoOwner(), baseRepo.RepoName())
	if err != nil {
		return nil
	}
	return remote
}

func mergeConflictStatus(status string) bool {
	return status == MergeStateStatusDirty
}

func isImmediatelyMergeable(status string) bool {
	switch status {
	case MergeStateStatusClean, MergeStateStatusHasHooks, MergeStateStatusUnstable:
		return true
	default:
		return false
	}
}
