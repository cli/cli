package merge

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/AlecAivazis/survey/v2"
	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/prompt"
	"github.com/cli/cli/v2/pkg/surveyext"
	"github.com/spf13/cobra"
)

type editor interface {
	Edit(string, string) (string, error)
}

type MergeOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	Branch     func() (string, error)
	Remotes    func() (context.Remotes, error)

	Finder shared.PRFinder

	SelectorArg  string
	DeleteBranch bool
	MergeMethod  PullRequestMergeMethod

	AutoMergeEnable  bool
	AutoMergeDisable bool

	Body    string
	BodySet bool
	Subject string
	Editor  editor

	UseAdmin                bool
	IsDeleteBranchIndicated bool
	CanDeleteLocalBranch    bool
	InteractiveMode         bool
}

func NewCmdMerge(f *cmdutil.Factory, runF func(*MergeOptions) error) *cobra.Command {
	opts := &MergeOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Branch:     f.Branch,
		Remotes:    f.Remotes,
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
				if !opts.IO.CanPrompt() {
					return cmdutil.FlagErrorf("--merge, --rebase, or --squash required when not running interactively")
				}
				opts.InteractiveMode = true
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
			return mergeRun(opts)
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
	return cmd
}

func mergeRun(opts *MergeOptions) error {
	cs := opts.IO.ColorScheme()

	findOptions := shared.FindOptions{
		Selector: opts.SelectorArg,
		Fields:   []string{"id", "number", "state", "title", "lastCommit", "mergeStateStatus", "headRepositoryOwner", "headRefName"},
	}
	pr, baseRepo, err := opts.Finder.Find(findOptions)
	if err != nil {
		return err
	}

	isTerminal := opts.IO.IsStdoutTTY()

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	apiClient := api.NewClientFromHTTP(httpClient)

	if opts.AutoMergeDisable {
		err := disableAutoMerge(httpClient, baseRepo, pr.ID)
		if err != nil {
			return err
		}
		if isTerminal {
			fmt.Fprintf(opts.IO.ErrOut, "%s Auto-merge disabled for pull request #%d\n", cs.SuccessIconWithColor(cs.Green), pr.Number)
		}
		return nil
	}

	if opts.SelectorArg == "" && len(pr.Commits.Nodes) > 0 {
		if localBranchLastCommit, err := git.LastCommit(); err == nil {
			if localBranchLastCommit.Sha != pr.Commits.Nodes[len(pr.Commits.Nodes)-1].Commit.OID {
				fmt.Fprintf(opts.IO.ErrOut,
					"%s Pull request #%d (%s) has diverged from local branch\n", cs.Yellow("!"), pr.Number, pr.Title)
			}
		}
	}

	isPRAlreadyMerged := pr.State == "MERGED"
	if reason := blockedReason(pr.MergeStateStatus, opts.UseAdmin); !opts.AutoMergeEnable && !isPRAlreadyMerged && reason != "" {
		fmt.Fprintf(opts.IO.ErrOut, "%s Pull request #%d is not mergeable: %s.\n", cs.FailureIcon(), pr.Number, reason)
		fmt.Fprintf(opts.IO.ErrOut, "To have the pull request merged after all the requirements have been met, add the `--auto` flag.\n")
		if !opts.UseAdmin && allowsAdminOverride(pr.MergeStateStatus) {
			// TODO: show this flag only to repo admins
			fmt.Fprintf(opts.IO.ErrOut, "To use administrator privileges to immediately merge the pull request, add the `--admin` flag.\n")
		}
		return cmdutil.SilentError
	}

	deleteBranch := opts.DeleteBranch
	crossRepoPR := pr.HeadRepositoryOwner.Login != baseRepo.RepoOwner()
	autoMerge := opts.AutoMergeEnable && !isImmediatelyMergeable(pr.MergeStateStatus)
	localBranchExists := false
	if opts.CanDeleteLocalBranch {
		localBranchExists = git.HasLocalBranch(pr.HeadRefName)
	}

	if !isPRAlreadyMerged {
		payload := mergePayload{
			repo:          baseRepo,
			pullRequestID: pr.ID,
			method:        opts.MergeMethod,
			auto:          autoMerge,
			commitSubject: opts.Subject,
			commitBody:    opts.Body,
			setCommitBody: opts.BodySet,
		}

		if opts.InteractiveMode {
			r, err := api.GitHubRepo(apiClient, baseRepo)
			if err != nil {
				return err
			}
			payload.method, err = mergeMethodSurvey(r)
			if err != nil {
				return err
			}
			deleteBranch, err = deleteBranchSurvey(opts, crossRepoPR, localBranchExists)
			if err != nil {
				return err
			}

			allowEditMsg := payload.method != PullRequestMergeMethodRebase

			for {
				action, err := confirmSurvey(allowEditMsg)
				if err != nil {
					return fmt.Errorf("unable to confirm: %w", err)
				}

				submit, err := confirmSubmission(httpClient, opts, action, &payload)
				if err != nil {
					return err
				}
				if submit {
					break
				}
			}
		}

		err = mergePullRequest(httpClient, payload)
		if err != nil {
			return err
		}

		if isTerminal {
			if payload.auto {
				method := ""
				switch payload.method {
				case PullRequestMergeMethodRebase:
					method = " via rebase"
				case PullRequestMergeMethodSquash:
					method = " via squash"
				}
				fmt.Fprintf(opts.IO.ErrOut, "%s Pull request #%d will be automatically merged%s when all requirements are met\n", cs.SuccessIconWithColor(cs.Green), pr.Number, method)
			} else {
				action := "Merged"
				switch payload.method {
				case PullRequestMergeMethodRebase:
					action = "Rebased and merged"
				case PullRequestMergeMethodSquash:
					action = "Squashed and merged"
				}
				fmt.Fprintf(opts.IO.ErrOut, "%s %s pull request #%d (%s)\n", cs.SuccessIconWithColor(cs.Magenta), action, pr.Number, pr.Title)
			}
		}
	} else if !opts.IsDeleteBranchIndicated && opts.InteractiveMode && !crossRepoPR && !opts.AutoMergeEnable {
		err := prompt.SurveyAskOne(&survey.Confirm{
			Message: fmt.Sprintf("Pull request #%d was already merged. Delete the branch locally?", pr.Number),
			Default: false,
		}, &deleteBranch)
		if err != nil {
			return fmt.Errorf("could not prompt: %w", err)
		}
	} else if crossRepoPR {
		fmt.Fprintf(opts.IO.ErrOut, "%s Pull request #%d was already merged\n", cs.WarningIcon(), pr.Number)
	}

	if !deleteBranch || crossRepoPR || autoMerge {
		return nil
	}

	branchSwitchString := ""

	if opts.CanDeleteLocalBranch && localBranchExists {
		currentBranch, err := opts.Branch()
		if err != nil {
			return err
		}

		var branchToSwitchTo string
		if currentBranch == pr.HeadRefName {
			branchToSwitchTo, err = api.RepoDefaultBranch(apiClient, baseRepo)
			if err != nil {
				return err
			}

			err = git.CheckoutBranch(branchToSwitchTo)
			if err != nil {
				return err
			}

			err := pullLatestChanges(opts, baseRepo, branchToSwitchTo)
			if err != nil {
				fmt.Fprintf(opts.IO.ErrOut, "%s warning: not posible to fast-forward to: %q\n", cs.WarningIcon(), branchToSwitchTo)
			}
		}

		if err := git.DeleteLocalBranch(pr.HeadRefName); err != nil {
			err = fmt.Errorf("failed to delete local branch %s: %w", cs.Cyan(pr.HeadRefName), err)
			return err
		}

		if branchToSwitchTo != "" {
			branchSwitchString = fmt.Sprintf(" and switched to branch %s", cs.Cyan(branchToSwitchTo))
		}
	}
	if !isPRAlreadyMerged {
		err = api.BranchDeleteRemote(apiClient, baseRepo, pr.HeadRefName)
		var httpErr api.HTTPError
		// The ref might have already been deleted by GitHub
		if err != nil && (!errors.As(err, &httpErr) || httpErr.StatusCode != 422) {
			err = fmt.Errorf("failed to delete remote branch %s: %w", cs.Cyan(pr.HeadRefName), err)
			return err
		}
	}

	if isTerminal {
		fmt.Fprintf(opts.IO.ErrOut, "%s Deleted branch %s%s\n", cs.SuccessIconWithColor(cs.Red), cs.Cyan(pr.HeadRefName), branchSwitchString)
	}

	return nil
}

func pullLatestChanges(opts *MergeOptions, repo ghrepo.Interface, branch string) error {
	remotes, err := opts.Remotes()
	if err != nil {
		return err
	}

	baseRemote, err := remotes.FindByRepo(repo.RepoOwner(), repo.RepoName())
	if err != nil {
		return err
	}

	err = git.Pull(baseRemote.Name, branch)
	if err != nil {
		return err
	}

	return nil
}

func mergeMethodSurvey(baseRepo *api.Repository) (PullRequestMergeMethod, error) {
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

	mergeQuestion := &survey.Select{
		Message: "What merge method would you like to use?",
		Options: surveyOpts,
	}

	var result int
	err := prompt.SurveyAskOne(mergeQuestion, &result)
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

		var result bool
		submit := &survey.Confirm{
			Message: message,
			Default: false,
		}
		err := prompt.SurveyAskOne(submit, &result)
		return result, err
	}

	return opts.DeleteBranch, nil
}

func confirmSurvey(allowEditMsg bool) (shared.Action, error) {
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

	var result string
	submit := &survey.Select{
		Message: "What's next?",
		Options: options,
	}
	err := prompt.SurveyAskOne(submit, &result)
	if err != nil {
		return shared.CancelAction, fmt.Errorf("could not prompt: %w", err)
	}

	switch result {
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
	case "BLOCKED":
		if useAdmin {
			return ""
		}
		return "the base branch policy prohibits the merge"
	case "BEHIND":
		if useAdmin {
			return ""
		}
		return "the head branch is not up to date with the base branch"
	case "DIRTY":
		return "the merge commit cannot be cleanly created"
	default:
		return ""
	}
}

func allowsAdminOverride(status string) bool {
	switch status {
	case "BLOCKED", "BEHIND":
		return true
	default:
		return false
	}
}

func isImmediatelyMergeable(status string) bool {
	switch status {
	case "CLEAN", "HAS_HOOKS", "UNSTABLE":
		return true
	default:
		return false
	}
}
