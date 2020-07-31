package command

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/api"
	"github.com/cli/cli/context"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmd/pr/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/prompt"
	"github.com/cli/cli/pkg/text"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func init() {
	prCmd.PersistentFlags().StringP("repo", "R", "", "Select another repository using the `OWNER/REPO` format")

	RootCmd.AddCommand(prCmd)
	prCmd.AddCommand(prCreateCmd)
	prCmd.AddCommand(prStatusCmd)
	prCmd.AddCommand(prCloseCmd)
	prCmd.AddCommand(prReopenCmd)
	prCmd.AddCommand(prMergeCmd)
	prMergeCmd.Flags().BoolP("delete-branch", "d", true, "Delete the local and remote branch after merge")
	prMergeCmd.Flags().BoolP("merge", "m", false, "Merge the commits with the base branch")
	prMergeCmd.Flags().BoolP("rebase", "r", false, "Rebase the commits onto the base branch")
	prMergeCmd.Flags().BoolP("squash", "s", false, "Squash the commits into one commit and merge it into the base branch")
	prCmd.AddCommand(prReadyCmd)

	prCmd.AddCommand(prListCmd)
	prListCmd.Flags().BoolP("web", "w", false, "Open the browser to list the pull request(s)")
	prListCmd.Flags().IntP("limit", "L", 30, "Maximum number of items to fetch")
	prListCmd.Flags().StringP("state", "s", "open", "Filter by state: {open|closed|merged|all}")
	prListCmd.Flags().StringP("base", "B", "", "Filter by base branch")
	prListCmd.Flags().StringSliceP("label", "l", nil, "Filter by labels")
	prListCmd.Flags().StringP("assignee", "a", "", "Filter by assignee")
}

var prCmd = &cobra.Command{
	Use:   "pr <command>",
	Short: "Create, view, and checkout pull requests",
	Long:  `Work with GitHub pull requests`,
	Example: heredoc.Doc(`
	$ gh pr checkout 353
	$ gh pr create --fill
	$ gh pr view --web
	`),
	Annotations: map[string]string{
		"IsCore": "true",
		"help:arguments": `A pull request can be supplied as argument in any of the following formats:
- by number, e.g. "123";
- by URL, e.g. "https://github.com/OWNER/REPO/pull/123"; or
- by the name of its head branch, e.g. "patch-1" or "OWNER:patch-1".`},
}
var prListCmd = &cobra.Command{
	Use:   "list",
	Short: "List and filter pull requests in this repository",
	Args:  cmdutil.NoArgsQuoteReminder,
	Example: heredoc.Doc(`
	$ gh pr list --limit 999
	$ gh pr list --state closed
	$ gh pr list --label "priority 1" --label "bug"
	$ gh pr list --web
	`),
	RunE: prList,
}
var prStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show status of relevant pull requests",
	Args:  cmdutil.NoArgsQuoteReminder,
	RunE:  prStatus,
}
var prCloseCmd = &cobra.Command{
	Use:   "close {<number> | <url> | <branch>}",
	Short: "Close a pull request",
	Args:  cobra.ExactArgs(1),
	RunE:  prClose,
}
var prReopenCmd = &cobra.Command{
	Use:   "reopen {<number> | <url> | <branch>}",
	Short: "Reopen a pull request",
	Args:  cobra.ExactArgs(1),
	RunE:  prReopen,
}
var prMergeCmd = &cobra.Command{
	Use:   "merge [<number> | <url> | <branch>]",
	Short: "Merge a pull request",
	Long: heredoc.Doc(`
	Merge a pull request on GitHub.

	By default, the head branch of the pull request will get deleted on both remote and local repositories.
	To retain the branch, use '--delete-branch=false'.
	`),
	Args: cobra.MaximumNArgs(1),
	RunE: prMerge,
}
var prReadyCmd = &cobra.Command{
	Use:   "ready [<number> | <url> | <branch>]",
	Short: "Mark a pull request as ready for review",
	Args:  cobra.MaximumNArgs(1),
	RunE:  prReady,
}

func prStatus(cmd *cobra.Command, args []string) error {
	ctx := contextForCommand(cmd)
	apiClient, err := apiClientForContext(ctx)
	if err != nil {
		return err
	}

	baseRepo, err := determineBaseRepo(apiClient, cmd, ctx)
	if err != nil {
		return err
	}

	repoOverride, _ := cmd.Flags().GetString("repo")
	currentPRNumber, currentPRHeadRef, err := prSelectorForCurrentBranch(ctx, baseRepo)

	if err != nil && repoOverride == "" && !errors.Is(err, git.ErrNotOnAnyBranch) {
		return fmt.Errorf("could not query for pull request for current branch: %w", err)
	}

	// the `@me` macro is available because the API lookup is ElasticSearch-based
	currentUser := "@me"
	prPayload, err := api.PullRequests(apiClient, baseRepo, currentPRNumber, currentPRHeadRef, currentUser)
	if err != nil {
		return err
	}

	out := colorableOut(cmd)

	fmt.Fprintln(out, "")
	fmt.Fprintf(out, "Relevant pull requests in %s\n", ghrepo.FullName(baseRepo))
	fmt.Fprintln(out, "")

	printHeader(out, "Current branch")
	currentPR := prPayload.CurrentPR
	currentBranch, _ := ctx.Branch()
	if currentPR != nil && currentPR.State != "OPEN" && prPayload.DefaultBranch == currentBranch {
		currentPR = nil
	}
	if currentPR != nil {
		printPrs(out, 1, *currentPR)
	} else if currentPRHeadRef == "" {
		printMessage(out, "  There is no current branch")
	} else {
		printMessage(out, fmt.Sprintf("  There is no pull request associated with %s", utils.Cyan("["+currentPRHeadRef+"]")))
	}
	fmt.Fprintln(out)

	printHeader(out, "Created by you")
	if prPayload.ViewerCreated.TotalCount > 0 {
		printPrs(out, prPayload.ViewerCreated.TotalCount, prPayload.ViewerCreated.PullRequests...)
	} else {
		printMessage(out, "  You have no open pull requests")
	}
	fmt.Fprintln(out)

	printHeader(out, "Requesting a code review from you")
	if prPayload.ReviewRequested.TotalCount > 0 {
		printPrs(out, prPayload.ReviewRequested.TotalCount, prPayload.ReviewRequested.PullRequests...)
	} else {
		printMessage(out, "  You have no pull requests to review")
	}
	fmt.Fprintln(out)

	return nil
}

func prList(cmd *cobra.Command, args []string) error {
	ctx := contextForCommand(cmd)
	apiClient, err := apiClientForContext(ctx)
	if err != nil {
		return err
	}

	baseRepo, err := determineBaseRepo(apiClient, cmd, ctx)
	if err != nil {
		return err
	}

	web, err := cmd.Flags().GetBool("web")
	if err != nil {
		return err
	}

	limit, err := cmd.Flags().GetInt("limit")
	if err != nil {
		return err
	}
	if limit <= 0 {
		return fmt.Errorf("invalid limit: %v", limit)
	}

	state, err := cmd.Flags().GetString("state")
	if err != nil {
		return err
	}
	baseBranch, err := cmd.Flags().GetString("base")
	if err != nil {
		return err
	}
	labels, err := cmd.Flags().GetStringSlice("label")
	if err != nil {
		return err
	}
	assignee, err := cmd.Flags().GetString("assignee")
	if err != nil {
		return err
	}

	if web {
		prListURL := ghrepo.GenerateRepoURL(baseRepo, "pulls")
		openURL, err := listURLWithQuery(prListURL, filterOptions{
			entity:     "pr",
			state:      state,
			assignee:   assignee,
			labels:     labels,
			baseBranch: baseBranch,
		})
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "Opening %s in your browser.\n", utils.DisplayURL(openURL))
		return utils.OpenInBrowser(openURL)
	}

	var graphqlState []string
	switch state {
	case "open":
		graphqlState = []string{"OPEN"}
	case "closed":
		graphqlState = []string{"CLOSED", "MERGED"}
	case "merged":
		graphqlState = []string{"MERGED"}
	case "all":
		graphqlState = []string{"OPEN", "CLOSED", "MERGED"}
	default:
		return fmt.Errorf("invalid state: %s", state)
	}

	params := map[string]interface{}{
		"owner": baseRepo.RepoOwner(),
		"repo":  baseRepo.RepoName(),
		"state": graphqlState,
	}
	if len(labels) > 0 {
		params["labels"] = labels
	}
	if baseBranch != "" {
		params["baseBranch"] = baseBranch
	}
	if assignee != "" {
		params["assignee"] = assignee
	}

	listResult, err := api.PullRequestList(apiClient, params, limit)
	if err != nil {
		return err
	}

	hasFilters := false
	cmd.Flags().Visit(func(f *pflag.Flag) {
		switch f.Name {
		case "state", "label", "base", "assignee":
			hasFilters = true
		}
	})

	title := listHeader(ghrepo.FullName(baseRepo), "pull request", len(listResult.PullRequests), listResult.TotalCount, hasFilters)
	if connectedToTerminal(cmd) {
		fmt.Fprintf(colorableErr(cmd), "\n%s\n\n", title)
	}

	table := utils.NewTablePrinter(cmd.OutOrStdout())
	for _, pr := range listResult.PullRequests {
		prNum := strconv.Itoa(pr.Number)
		if table.IsTTY() {
			prNum = "#" + prNum
		}
		table.AddField(prNum, nil, shared.ColorFuncForPR(pr))
		table.AddField(replaceExcessiveWhitespace(pr.Title), nil, nil)
		table.AddField(pr.HeadLabel(), nil, utils.Cyan)
		if !table.IsTTY() {
			table.AddField(prStateWithDraft(&pr), nil, nil)
		}
		table.EndRow()
	}
	err = table.Render()
	if err != nil {
		return err
	}

	return nil
}

func prClose(cmd *cobra.Command, args []string) error {
	ctx := contextForCommand(cmd)
	apiClient, err := apiClientForContext(ctx)
	if err != nil {
		return err
	}

	pr, baseRepo, err := prFromArgs(ctx, apiClient, cmd, args)
	if err != nil {
		return err
	}

	if pr.State == "MERGED" {
		err := fmt.Errorf("%s Pull request #%d (%s) can't be closed because it was already merged", utils.Red("!"), pr.Number, pr.Title)
		return err
	} else if pr.Closed {
		fmt.Fprintf(colorableErr(cmd), "%s Pull request #%d (%s) is already closed\n", utils.Yellow("!"), pr.Number, pr.Title)
		return nil
	}

	err = api.PullRequestClose(apiClient, baseRepo, pr)
	if err != nil {
		return fmt.Errorf("API call failed: %w", err)
	}

	fmt.Fprintf(colorableErr(cmd), "%s Closed pull request #%d (%s)\n", utils.Red("✔"), pr.Number, pr.Title)

	return nil
}

func prReopen(cmd *cobra.Command, args []string) error {
	ctx := contextForCommand(cmd)
	apiClient, err := apiClientForContext(ctx)
	if err != nil {
		return err
	}

	pr, baseRepo, err := prFromArgs(ctx, apiClient, cmd, args)
	if err != nil {
		return err
	}

	if pr.State == "MERGED" {
		err := fmt.Errorf("%s Pull request #%d (%s) can't be reopened because it was already merged", utils.Red("!"), pr.Number, pr.Title)
		return err
	}

	if !pr.Closed {
		fmt.Fprintf(colorableErr(cmd), "%s Pull request #%d (%s) is already open\n", utils.Yellow("!"), pr.Number, pr.Title)
		return nil
	}

	err = api.PullRequestReopen(apiClient, baseRepo, pr)
	if err != nil {
		return fmt.Errorf("API call failed: %w", err)
	}

	fmt.Fprintf(colorableErr(cmd), "%s Reopened pull request #%d (%s)\n", utils.Green("✔"), pr.Number, pr.Title)

	return nil
}

func prMerge(cmd *cobra.Command, args []string) error {
	ctx := contextForCommand(cmd)
	apiClient, err := apiClientForContext(ctx)
	if err != nil {
		return err
	}

	pr, baseRepo, err := prFromArgs(ctx, apiClient, cmd, args)
	if err != nil {
		return err
	}

	if pr.Mergeable == "CONFLICTING" {
		err := fmt.Errorf("%s Pull request #%d (%s) has conflicts and isn't mergeable ", utils.Red("!"), pr.Number, pr.Title)
		return err
	} else if pr.Mergeable == "UNKNOWN" {
		err := fmt.Errorf("%s Pull request #%d (%s) can't be merged right now; try again in a few seconds", utils.Red("!"), pr.Number, pr.Title)
		return err
	} else if pr.State == "MERGED" {
		err := fmt.Errorf("%s Pull request #%d (%s) was already merged", utils.Red("!"), pr.Number, pr.Title)
		return err
	}

	var mergeMethod api.PullRequestMergeMethod
	deleteBranch, err := cmd.Flags().GetBool("delete-branch")
	if err != nil {
		return err
	}

	deleteLocalBranch := !cmd.Flags().Changed("repo")
	crossRepoPR := pr.HeadRepositoryOwner.Login != baseRepo.RepoOwner()

	// Ensure only one merge method is specified
	enabledFlagCount := 0
	isInteractive := false
	if b, _ := cmd.Flags().GetBool("merge"); b {
		enabledFlagCount++
		mergeMethod = api.PullRequestMergeMethodMerge
	}
	if b, _ := cmd.Flags().GetBool("rebase"); b {
		enabledFlagCount++
		mergeMethod = api.PullRequestMergeMethodRebase
	}
	if b, _ := cmd.Flags().GetBool("squash"); b {
		enabledFlagCount++
		mergeMethod = api.PullRequestMergeMethodSquash
	}

	if enabledFlagCount == 0 {
		if !connectedToTerminal(cmd) {
			return errors.New("--merge, --rebase, or --squash required when not attached to a tty")
		}
		isInteractive = true
	} else if enabledFlagCount > 1 {
		return errors.New("expected exactly one of --merge, --rebase, or --squash to be true")
	}

	if isInteractive {
		mergeMethod, deleteBranch, err = prInteractiveMerge(deleteLocalBranch, crossRepoPR)
		if err != nil {
			return nil
		}
	}

	var action string
	if mergeMethod == api.PullRequestMergeMethodRebase {
		action = "Rebased and merged"
		err = api.PullRequestMerge(apiClient, baseRepo, pr, api.PullRequestMergeMethodRebase)
	} else if mergeMethod == api.PullRequestMergeMethodSquash {
		action = "Squashed and merged"
		err = api.PullRequestMerge(apiClient, baseRepo, pr, api.PullRequestMergeMethodSquash)
	} else if mergeMethod == api.PullRequestMergeMethodMerge {
		action = "Merged"
		err = api.PullRequestMerge(apiClient, baseRepo, pr, api.PullRequestMergeMethodMerge)
	} else {
		err = fmt.Errorf("unknown merge method (%d) used", mergeMethod)
		return err
	}

	if err != nil {
		return fmt.Errorf("API call failed: %w", err)
	}

	if connectedToTerminal(cmd) {
		fmt.Fprintf(colorableErr(cmd), "%s %s pull request #%d (%s)\n", utils.Magenta("✔"), action, pr.Number, pr.Title)
	}

	if deleteBranch {
		branchSwitchString := ""

		if deleteLocalBranch && !crossRepoPR {
			currentBranch, err := ctx.Branch()
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
			}

			localBranchExists := git.HasLocalBranch(pr.HeadRefName)
			if localBranchExists {
				err = git.DeleteLocalBranch(pr.HeadRefName)
				if err != nil {
					err = fmt.Errorf("failed to delete local branch %s: %w", utils.Cyan(pr.HeadRefName), err)
					return err
				}
			}

			if branchToSwitchTo != "" {
				branchSwitchString = fmt.Sprintf(" and switched to branch %s", utils.Cyan(branchToSwitchTo))
			}
		}

		if !crossRepoPR {
			err = api.BranchDeleteRemote(apiClient, baseRepo, pr.HeadRefName)
			var httpErr api.HTTPError
			// The ref might have already been deleted by GitHub
			if err != nil && (!errors.As(err, &httpErr) || httpErr.StatusCode != 422) {
				err = fmt.Errorf("failed to delete remote branch %s: %w", utils.Cyan(pr.HeadRefName), err)
				return err
			}
		}

		if connectedToTerminal(cmd) {
			fmt.Fprintf(colorableErr(cmd), "%s Deleted branch %s%s\n", utils.Red("✔"), utils.Cyan(pr.HeadRefName), branchSwitchString)
		}
	}

	return nil
}

func prInteractiveMerge(deleteLocalBranch bool, crossRepoPR bool) (api.PullRequestMergeMethod, bool, error) {
	mergeMethodQuestion := &survey.Question{
		Name: "mergeMethod",
		Prompt: &survey.Select{
			Message: "What merge method would you like to use?",
			Options: []string{"Create a merge commit", "Rebase and merge", "Squash and merge"},
			Default: "Create a merge commit",
		},
	}

	qs := []*survey.Question{mergeMethodQuestion}

	if !crossRepoPR {
		var message string
		if deleteLocalBranch {
			message = "Delete the branch locally and on GitHub?"
		} else {
			message = "Delete the branch on GitHub?"
		}

		deleteBranchQuestion := &survey.Question{
			Name: "deleteBranch",
			Prompt: &survey.Confirm{
				Message: message,
				Default: true,
			},
		}
		qs = append(qs, deleteBranchQuestion)
	}

	answers := struct {
		MergeMethod  int
		DeleteBranch bool
	}{}

	err := prompt.SurveyAsk(qs, &answers)
	if err != nil {
		return 0, false, fmt.Errorf("could not prompt: %w", err)
	}

	var mergeMethod api.PullRequestMergeMethod
	switch answers.MergeMethod {
	case 0:
		mergeMethod = api.PullRequestMergeMethodMerge
	case 1:
		mergeMethod = api.PullRequestMergeMethodRebase
	case 2:
		mergeMethod = api.PullRequestMergeMethodSquash
	}

	deleteBranch := answers.DeleteBranch
	return mergeMethod, deleteBranch, nil
}

func prStateWithDraft(pr *api.PullRequest) string {
	if pr.IsDraft && pr.State == "OPEN" {
		return "DRAFT"
	}

	return pr.State
}

func prReady(cmd *cobra.Command, args []string) error {
	ctx := contextForCommand(cmd)
	apiClient, err := apiClientForContext(ctx)
	if err != nil {
		return err
	}

	pr, baseRepo, err := prFromArgs(ctx, apiClient, cmd, args)
	if err != nil {
		return err
	}

	if pr.Closed {
		err := fmt.Errorf("%s Pull request #%d is closed. Only draft pull requests can be marked as \"ready for review\"", utils.Red("!"), pr.Number)
		return err
	} else if !pr.IsDraft {
		fmt.Fprintf(colorableErr(cmd), "%s Pull request #%d is already \"ready for review\"\n", utils.Yellow("!"), pr.Number)
		return nil
	}

	err = api.PullRequestReady(apiClient, baseRepo, pr)
	if err != nil {
		return fmt.Errorf("API call failed: %w", err)
	}

	fmt.Fprintf(colorableErr(cmd), "%s Pull request #%d is marked as \"ready for review\"\n", utils.Green("✔"), pr.Number)

	return nil
}

func prSelectorForCurrentBranch(ctx context.Context, baseRepo ghrepo.Interface) (prNumber int, prHeadRef string, err error) {
	prHeadRef, err = ctx.Branch()
	if err != nil {
		return
	}
	branchConfig := git.ReadBranchConfig(prHeadRef)

	// the branch is configured to merge a special PR head ref
	prHeadRE := regexp.MustCompile(`^refs/pull/(\d+)/head$`)
	if m := prHeadRE.FindStringSubmatch(branchConfig.MergeRef); m != nil {
		prNumber, _ = strconv.Atoi(m[1])
		return
	}

	var branchOwner string
	if branchConfig.RemoteURL != nil {
		// the branch merges from a remote specified by URL
		if r, err := ghrepo.FromURL(branchConfig.RemoteURL); err == nil {
			branchOwner = r.RepoOwner()
		}
	} else if branchConfig.RemoteName != "" {
		// the branch merges from a remote specified by name
		rem, _ := ctx.Remotes()
		if r, err := rem.FindByName(branchConfig.RemoteName); err == nil {
			branchOwner = r.RepoOwner()
		}
	}

	if branchOwner != "" {
		if strings.HasPrefix(branchConfig.MergeRef, "refs/heads/") {
			prHeadRef = strings.TrimPrefix(branchConfig.MergeRef, "refs/heads/")
		}
		// prepend `OWNER:` if this branch is pushed to a fork
		if !strings.EqualFold(branchOwner, baseRepo.RepoOwner()) {
			prHeadRef = fmt.Sprintf("%s:%s", branchOwner, prHeadRef)
		}
	}

	return
}

func printPrs(w io.Writer, totalCount int, prs ...api.PullRequest) {
	for _, pr := range prs {
		prNumber := fmt.Sprintf("#%d", pr.Number)

		prStateColorFunc := utils.Green
		if pr.IsDraft {
			prStateColorFunc = utils.Gray
		} else if pr.State == "MERGED" {
			prStateColorFunc = utils.Magenta
		} else if pr.State == "CLOSED" {
			prStateColorFunc = utils.Red
		}

		fmt.Fprintf(w, "  %s  %s %s", prStateColorFunc(prNumber), text.Truncate(50, replaceExcessiveWhitespace(pr.Title)), utils.Cyan("["+pr.HeadLabel()+"]"))

		checks := pr.ChecksStatus()
		reviews := pr.ReviewStatus()

		if pr.State == "OPEN" {
			reviewStatus := reviews.ChangesRequested || reviews.Approved || reviews.ReviewRequired
			if checks.Total > 0 || reviewStatus {
				// show checks & reviews on their own line
				fmt.Fprintf(w, "\n  ")
			}

			if checks.Total > 0 {
				var summary string
				if checks.Failing > 0 {
					if checks.Failing == checks.Total {
						summary = utils.Red("× All checks failing")
					} else {
						summary = utils.Red(fmt.Sprintf("× %d/%d checks failing", checks.Failing, checks.Total))
					}
				} else if checks.Pending > 0 {
					summary = utils.Yellow("- Checks pending")
				} else if checks.Passing == checks.Total {
					summary = utils.Green("✓ Checks passing")
				}
				fmt.Fprint(w, summary)
			}

			if checks.Total > 0 && reviewStatus {
				// add padding between checks & reviews
				fmt.Fprint(w, " ")
			}

			if reviews.ChangesRequested {
				fmt.Fprint(w, utils.Red("+ Changes requested"))
			} else if reviews.ReviewRequired {
				fmt.Fprint(w, utils.Yellow("- Review required"))
			} else if reviews.Approved {
				fmt.Fprint(w, utils.Green("✓ Approved"))
			}
		} else {
			fmt.Fprintf(w, " - %s", shared.StateTitleWithColor(pr))
		}

		fmt.Fprint(w, "\n")
	}
	remaining := totalCount - len(prs)
	if remaining > 0 {
		fmt.Fprintf(w, utils.Gray("  And %d more\n"), remaining)
	}
}

func printHeader(w io.Writer, s string) {
	fmt.Fprintln(w, utils.Bold(s))
}

func printMessage(w io.Writer, s string) {
	fmt.Fprintln(w, utils.Gray(s))
}

func replaceExcessiveWhitespace(s string) string {
	s = strings.TrimSpace(s)
	s = regexp.MustCompile(`\r?\n`).ReplaceAllString(s, " ")
	s = regexp.MustCompile(`\s{2,}`).ReplaceAllString(s, " ")
	return s
}
