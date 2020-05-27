package command

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/cli/cli/api"
	"github.com/cli/cli/context"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/text"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func init() {
	RootCmd.AddCommand(prCmd)
	prCmd.AddCommand(prCheckoutCmd)
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
	prListCmd.Flags().IntP("limit", "L", 30, "Maximum number of items to fetch")
	prListCmd.Flags().StringP("state", "s", "open", "Filter by state: {open|closed|merged|all}")
	prListCmd.Flags().StringP("base", "B", "", "Filter by base branch")
	prListCmd.Flags().StringSliceP("label", "l", nil, "Filter by label")
	prListCmd.Flags().StringP("assignee", "a", "", "Filter by assignee")

	prCmd.AddCommand(prViewCmd)
	prViewCmd.Flags().BoolP("web", "w", false, "Open a pull request in the browser")
}

var prCmd = &cobra.Command{
	Use:   "pr",
	Short: "Create, view, and checkout pull requests",
	Long: `Work with GitHub pull requests.

A pull request can be supplied as argument in any of the following formats:
- by number, e.g. "123";
- by URL, e.g. "https://github.com/OWNER/REPO/pull/123"; or
- by the name of its head branch, e.g. "patch-1" or "OWNER:patch-1".`,
}
var prListCmd = &cobra.Command{
	Use:   "list",
	Short: "List and filter pull requests in this repository",
	RunE:  prList,
}
var prStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show status of relevant pull requests",
	RunE:  prStatus,
}
var prViewCmd = &cobra.Command{
	Use:   "view [<number> | <url> | <branch>]",
	Short: "View a pull request",
	Long: `Display the title, body, and other information about a pull request.

Without an argument, the pull request that belongs to the current branch
is displayed.

With '--web', open the pull request in a web browser instead.`,
	RunE: prView,
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
	Args:  cobra.MaximumNArgs(1),
	RunE:  prMerge,
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

	if err != nil && repoOverride == "" && err.Error() != "git: not on any branch" {
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

	limit, err := cmd.Flags().GetInt("limit")
	if err != nil {
		return err
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
	// TODO: avoid printing header if piped to a script
	fmt.Fprintf(colorableErr(cmd), "\n%s\n\n", title)

	table := utils.NewTablePrinter(cmd.OutOrStdout())
	for _, pr := range listResult.PullRequests {
		prNum := strconv.Itoa(pr.Number)
		if table.IsTTY() {
			prNum = "#" + prNum
		}
		table.AddField(prNum, nil, colorFuncForPR(pr))
		table.AddField(replaceExcessiveWhitespace(pr.Title), nil, nil)
		table.AddField(pr.HeadLabel(), nil, utils.Cyan)
		table.EndRow()
	}
	err = table.Render()
	if err != nil {
		return err
	}

	return nil
}

func prStateTitleWithColor(pr api.PullRequest) string {
	prStateColorFunc := colorFuncForPR(pr)
	if pr.State == "OPEN" && pr.IsDraft {
		return prStateColorFunc(strings.Title(strings.ToLower("Draft")))
	}
	return prStateColorFunc(strings.Title(strings.ToLower(pr.State)))
}

func colorFuncForPR(pr api.PullRequest) func(string) string {
	if pr.State == "OPEN" && pr.IsDraft {
		return utils.Gray
	}
	return colorFuncForState(pr.State)
}

// colorFuncForState returns a color function for a PR/Issue state
func colorFuncForState(state string) func(string) string {
	switch state {
	case "OPEN":
		return utils.Green
	case "CLOSED":
		return utils.Red
	case "MERGED":
		return utils.Magenta
	default:
		return nil
	}
}

func prView(cmd *cobra.Command, args []string) error {
	ctx := contextForCommand(cmd)

	apiClient, err := apiClientForContext(ctx)
	if err != nil {
		return err
	}

	var baseRepo ghrepo.Interface
	var prArg string
	if len(args) > 0 {
		prArg = args[0]
		if prNum, repo := prFromURL(prArg); repo != nil {
			prArg = prNum
			baseRepo = repo
		}
	}

	if baseRepo == nil {
		baseRepo, err = determineBaseRepo(apiClient, cmd, ctx)
		if err != nil {
			return err
		}
	}

	web, err := cmd.Flags().GetBool("web")
	if err != nil {
		return err
	}

	var openURL string
	var pr *api.PullRequest
	if len(args) > 0 {
		pr, err = prFromArg(apiClient, baseRepo, prArg)
		if err != nil {
			return err
		}
		openURL = pr.URL
	} else {
		prNumber, branchWithOwner, err := prSelectorForCurrentBranch(ctx, baseRepo)
		if err != nil {
			return err
		}

		if prNumber > 0 {
			openURL = fmt.Sprintf("https://github.com/%s/pull/%d", ghrepo.FullName(baseRepo), prNumber)
			if !web {
				pr, err = api.PullRequestByNumber(apiClient, baseRepo, prNumber)
				if err != nil {
					return err
				}
			}
		} else {
			pr, err = api.PullRequestForBranch(apiClient, baseRepo, "", branchWithOwner)
			if err != nil {
				return err
			}

			openURL = pr.URL
		}
	}

	if web {
		fmt.Fprintf(cmd.ErrOrStderr(), "Opening %s in your browser.\n", openURL)
		return utils.OpenInBrowser(openURL)
	} else {
		out := colorableOut(cmd)
		return printPrPreview(out, pr)
	}
}

func prClose(cmd *cobra.Command, args []string) error {
	ctx := contextForCommand(cmd)
	apiClient, err := apiClientForContext(ctx)
	if err != nil {
		return err
	}

	baseRepo, err := determineBaseRepo(apiClient, cmd, ctx)
	if err != nil {
		return err
	}

	pr, err := prFromArg(apiClient, baseRepo, args[0])
	if err != nil {
		return err
	}

	if pr.State == "MERGED" {
		err := fmt.Errorf("%s Pull request #%d can't be closed because it was already merged", utils.Red("!"), pr.Number)
		return err
	} else if pr.Closed {
		fmt.Fprintf(colorableErr(cmd), "%s Pull request #%d is already closed\n", utils.Yellow("!"), pr.Number)
		return nil
	}

	err = api.PullRequestClose(apiClient, baseRepo, pr)
	if err != nil {
		return fmt.Errorf("API call failed: %w", err)
	}

	fmt.Fprintf(colorableErr(cmd), "%s Closed pull request #%d\n", utils.Red("✔"), pr.Number)

	return nil
}

func prReopen(cmd *cobra.Command, args []string) error {
	ctx := contextForCommand(cmd)
	apiClient, err := apiClientForContext(ctx)
	if err != nil {
		return err
	}

	baseRepo, err := determineBaseRepo(apiClient, cmd, ctx)
	if err != nil {
		return err
	}

	pr, err := prFromArg(apiClient, baseRepo, args[0])
	if err != nil {
		return err
	}

	if pr.State == "MERGED" {
		err := fmt.Errorf("%s Pull request #%d can't be reopened because it was already merged", utils.Red("!"), pr.Number)
		return err
	}

	if !pr.Closed {
		fmt.Fprintf(colorableErr(cmd), "%s Pull request #%d is already open\n", utils.Yellow("!"), pr.Number)
		return nil
	}

	err = api.PullRequestReopen(apiClient, baseRepo, pr)
	if err != nil {
		return fmt.Errorf("API call failed: %w", err)
	}

	fmt.Fprintf(colorableErr(cmd), "%s Reopened pull request #%d\n", utils.Green("✔"), pr.Number)

	return nil
}

func prMerge(cmd *cobra.Command, args []string) error {
	ctx := contextForCommand(cmd)
	apiClient, err := apiClientForContext(ctx)
	if err != nil {
		return err
	}

	baseRepo, err := determineBaseRepo(apiClient, cmd, ctx)
	if err != nil {
		return err
	}

	var pr *api.PullRequest
	if len(args) > 0 {
		var prNumber string
		n, _ := prFromURL(args[0])
		if n != "" {
			prNumber = n
		} else {
			prNumber = args[0]
		}

		pr, err = prFromArg(apiClient, baseRepo, prNumber)
		if err != nil {
			return err
		}
	} else {
		prNumber, branchWithOwner, err := prSelectorForCurrentBranch(ctx, baseRepo)
		if err != nil {
			return err
		}

		if prNumber != 0 {
			pr, err = api.PullRequestByNumber(apiClient, baseRepo, prNumber)
		} else {
			pr, err = api.PullRequestForBranch(apiClient, baseRepo, "", branchWithOwner)
		}
		if err != nil {
			return err
		}
	}

	if pr.Mergeable == "CONFLICTING" {
		err := fmt.Errorf("%s Pull request #%d has conflicts and isn't mergeable ", utils.Red("!"), pr.Number)
		return err
	} else if pr.Mergeable == "UNKNOWN" {
		err := fmt.Errorf("%s Pull request #%d can't be merged right now; try again in a few seconds", utils.Red("!"), pr.Number)
		return err
	} else if pr.State == "MERGED" {
		err := fmt.Errorf("%s Pull request #%d was already merged", utils.Red("!"), pr.Number)
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

	fmt.Fprintf(colorableOut(cmd), "%s %s pull request #%d\n", utils.Magenta("✔"), action, pr.Number)

	if deleteBranch {
		repo, err := api.GitHubRepo(apiClient, baseRepo)
		if err != nil {
			return err
		}

		currentBranch, err := ctx.Branch()
		if err != nil {
			return err
		}

		branchSwitchString := ""

		if deleteLocalBranch && !crossRepoPR {
			var branchToSwitchTo string
			if currentBranch == pr.HeadRefName {
				branchToSwitchTo = repo.DefaultBranchRef.Name
				err = git.CheckoutBranch(repo.DefaultBranchRef.Name)
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
			if err != nil {
				err = fmt.Errorf("failed to delete remote branch %s: %w", utils.Cyan(pr.HeadRefName), err)
				return err
			}
		}

		fmt.Fprintf(colorableOut(cmd), "%s Deleted branch %s%s\n", utils.Red("✔"), utils.Cyan(pr.HeadRefName), branchSwitchString)
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

	err := SurveyAsk(qs, &answers)
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

func printPrPreview(out io.Writer, pr *api.PullRequest) error {
	// Header (Title and State)
	fmt.Fprintln(out, utils.Bold(pr.Title))
	fmt.Fprintf(out, "%s", prStateTitleWithColor(*pr))
	fmt.Fprintln(out, utils.Gray(fmt.Sprintf(
		" • %s wants to merge %s into %s from %s",
		pr.Author.Login,
		utils.Pluralize(pr.Commits.TotalCount, "commit"),
		pr.BaseRefName,
		pr.HeadRefName,
	)))
	fmt.Fprintln(out)

	// Metadata
	if reviewers := prReviewerList(*pr); reviewers != "" {
		fmt.Fprint(out, utils.Bold("Reviewers: "))
		fmt.Fprintln(out, reviewers)
	}
	if assignees := prAssigneeList(*pr); assignees != "" {
		fmt.Fprint(out, utils.Bold("Assignees: "))
		fmt.Fprintln(out, assignees)
	}
	if labels := prLabelList(*pr); labels != "" {
		fmt.Fprint(out, utils.Bold("Labels: "))
		fmt.Fprintln(out, labels)
	}
	if projects := prProjectList(*pr); projects != "" {
		fmt.Fprint(out, utils.Bold("Projects: "))
		fmt.Fprintln(out, projects)
	}
	if pr.Milestone.Title != "" {
		fmt.Fprint(out, utils.Bold("Milestone: "))
		fmt.Fprintln(out, pr.Milestone.Title)
	}

	// Body
	if pr.Body != "" {
		fmt.Fprintln(out)
		md, err := utils.RenderMarkdown(pr.Body)
		if err != nil {
			return err
		}
		fmt.Fprintln(out, md)
	}
	fmt.Fprintln(out)

	// Footer
	fmt.Fprintf(out, utils.Gray("View this pull request on GitHub: %s\n"), pr.URL)
	return nil
}

func prReady(cmd *cobra.Command, args []string) error {
	ctx := contextForCommand(cmd)
	apiClient, err := apiClientForContext(ctx)
	if err != nil {
		return err
	}

	baseRepo, err := determineBaseRepo(apiClient, cmd, ctx)
	if err != nil {
		return err
	}

	var pr *api.PullRequest
	if len(args) > 0 {
		var prNumber string
		n, _ := prFromURL(args[0])
		if n != "" {
			prNumber = n
		} else {
			prNumber = args[0]
		}

		pr, err = prFromArg(apiClient, baseRepo, prNumber)
		if err != nil {
			return err
		}
	} else {
		prNumber, branchWithOwner, err := prSelectorForCurrentBranch(ctx, baseRepo)
		if err != nil {
			return err
		}

		if prNumber != 0 {
			pr, err = api.PullRequestByNumber(apiClient, baseRepo, prNumber)
		} else {
			pr, err = api.PullRequestForBranch(apiClient, baseRepo, "", branchWithOwner)
		}
		if err != nil {
			return err
		}
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

// Ref. https://developer.github.com/v4/enum/pullrequestreviewstate/
const (
	requestedReviewState        = "REQUESTED" // This is our own state for review request
	approvedReviewState         = "APPROVED"
	changesRequestedReviewState = "CHANGES_REQUESTED"
	commentedReviewState        = "COMMENTED"
	dismissedReviewState        = "DISMISSED"
	pendingReviewState          = "PENDING"
)

type reviewerState struct {
	Name  string
	State string
}

// colorFuncForReviewerState returns a color function for a reviewer state
func colorFuncForReviewerState(state string) func(string) string {
	switch state {
	case requestedReviewState:
		return utils.Yellow
	case approvedReviewState:
		return utils.Green
	case changesRequestedReviewState:
		return utils.Red
	case commentedReviewState:
		return func(str string) string { return str } // Do nothing
	default:
		return nil
	}
}

// formattedReviewerState formats a reviewerState with state color
func formattedReviewerState(reviewer *reviewerState) string {
	state := reviewer.State
	if state == dismissedReviewState {
		// Show "DISMISSED" review as "COMMENTED", since "dimissed" only makes
		// sense when displayed in an events timeline but not in the final tally.
		state = commentedReviewState
	}
	stateColorFunc := colorFuncForReviewerState(state)
	return fmt.Sprintf("%s (%s)", reviewer.Name, stateColorFunc(strings.ReplaceAll(strings.Title(strings.ToLower(state)), "_", " ")))
}

// prReviewerList generates a reviewer list with their last state
func prReviewerList(pr api.PullRequest) string {
	reviewerStates := parseReviewers(pr)
	reviewers := make([]string, 0, len(reviewerStates))

	sortReviewerStates(reviewerStates)

	for _, reviewer := range reviewerStates {
		reviewers = append(reviewers, formattedReviewerState(reviewer))
	}

	reviewerList := strings.Join(reviewers, ", ")

	return reviewerList
}

// Ref. https://developer.github.com/v4/union/requestedreviewer/
const teamTypeName = "Team"

const ghostName = "ghost"

// parseReviewers parses given Reviews and ReviewRequests
func parseReviewers(pr api.PullRequest) []*reviewerState {
	reviewerStates := make(map[string]*reviewerState)

	for _, review := range pr.Reviews.Nodes {
		if review.Author.Login != pr.Author.Login {
			name := review.Author.Login
			if name == "" {
				name = ghostName
			}
			reviewerStates[name] = &reviewerState{
				Name:  name,
				State: review.State,
			}
		}
	}

	// Overwrite reviewer's state if a review request for the same reviewer exists.
	for _, reviewRequest := range pr.ReviewRequests.Nodes {
		name := reviewRequest.RequestedReviewer.Login
		if reviewRequest.RequestedReviewer.TypeName == teamTypeName {
			name = reviewRequest.RequestedReviewer.Name
		}
		reviewerStates[name] = &reviewerState{
			Name:  name,
			State: requestedReviewState,
		}
	}

	// Convert map to slice for ease of sort
	result := make([]*reviewerState, 0, len(reviewerStates))
	for _, reviewer := range reviewerStates {
		if reviewer.State == pendingReviewState {
			continue
		}
		result = append(result, reviewer)
	}

	return result
}

// sortReviewerStates puts completed reviews before review requests and sorts names alphabetically
func sortReviewerStates(reviewerStates []*reviewerState) {
	sort.Slice(reviewerStates, func(i, j int) bool {
		if reviewerStates[i].State == requestedReviewState &&
			reviewerStates[j].State != requestedReviewState {
			return false
		}
		if reviewerStates[j].State == requestedReviewState &&
			reviewerStates[i].State != requestedReviewState {
			return true
		}

		return reviewerStates[i].Name < reviewerStates[j].Name
	})
}

func prAssigneeList(pr api.PullRequest) string {
	if len(pr.Assignees.Nodes) == 0 {
		return ""
	}

	AssigneeNames := make([]string, 0, len(pr.Assignees.Nodes))
	for _, assignee := range pr.Assignees.Nodes {
		AssigneeNames = append(AssigneeNames, assignee.Login)
	}

	list := strings.Join(AssigneeNames, ", ")
	if pr.Assignees.TotalCount > len(pr.Assignees.Nodes) {
		list += ", …"
	}
	return list
}

func prLabelList(pr api.PullRequest) string {
	if len(pr.Labels.Nodes) == 0 {
		return ""
	}

	labelNames := make([]string, 0, len(pr.Labels.Nodes))
	for _, label := range pr.Labels.Nodes {
		labelNames = append(labelNames, label.Name)
	}

	list := strings.Join(labelNames, ", ")
	if pr.Labels.TotalCount > len(pr.Labels.Nodes) {
		list += ", …"
	}
	return list
}

func prProjectList(pr api.PullRequest) string {
	if len(pr.ProjectCards.Nodes) == 0 {
		return ""
	}

	projectNames := make([]string, 0, len(pr.ProjectCards.Nodes))
	for _, project := range pr.ProjectCards.Nodes {
		colName := project.Column.Name
		if colName == "" {
			colName = "Awaiting triage"
		}
		projectNames = append(projectNames, fmt.Sprintf("%s (%s)", project.Project.Name, colName))
	}

	list := strings.Join(projectNames, ", ")
	if pr.ProjectCards.TotalCount > len(pr.ProjectCards.Nodes) {
		list += ", …"
	}
	return list
}

var prURLRE = regexp.MustCompile(`^https://github\.com/([^/]+)/([^/]+)/pull/(\d+)`)

func prFromURL(arg string) (string, ghrepo.Interface) {
	if m := prURLRE.FindStringSubmatch(arg); m != nil {
		return m[3], ghrepo.New(m[1], m[2])
	}
	return "", nil
}

func prFromArg(apiClient *api.Client, baseRepo ghrepo.Interface, arg string) (*api.PullRequest, error) {
	if prNumber, err := strconv.Atoi(strings.TrimPrefix(arg, "#")); err == nil {
		return api.PullRequestByNumber(apiClient, baseRepo, prNumber)
	}

	return api.PullRequestForBranch(apiClient, baseRepo, "", arg)
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
			fmt.Fprintf(w, " - %s", prStateTitleWithColor(pr))
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
