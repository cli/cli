package command

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"github.com/github/gh-cli/api"
	"github.com/github/gh-cli/git"
	"github.com/github/gh-cli/utils"
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(prCmd)
	prCmd.AddCommand(prCheckoutCmd)
	prCmd.AddCommand(prCreateCmd)
	prCmd.AddCommand(prListCmd)
	prCmd.AddCommand(prStatusCmd)
	prCmd.AddCommand(prViewCmd)

	prListCmd.Flags().IntP("limit", "L", 30, "maximum number of items to fetch")
	prListCmd.Flags().StringP("state", "s", "open", "filter by state")
	prListCmd.Flags().StringP("base", "b", "", "filter by base branch")
	prListCmd.Flags().StringArrayP("label", "l", nil, "filter by label")
}

var prCmd = &cobra.Command{
	Use:   "pr",
	Short: "Work with pull requests",
	Long:  `Work with GitHub pull requests.`,
}
var prCheckoutCmd = &cobra.Command{
	Use:   "checkout <pr-number>",
	Short: "Check out a pull request in Git",
	Args:  cobra.MinimumNArgs(1),
	RunE:  prCheckout,
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
	Use:   "view [pr-number]",
	Short: "View a pull request in the browser",
	RunE:  prView,
}

func prStatus(cmd *cobra.Command, args []string) error {
	ctx := contextForCommand(cmd)
	apiClient, err := apiClientForContext(ctx)
	if err != nil {
		return err
	}

	baseRepo, err := ctx.BaseRepo()
	if err != nil {
		return err
	}
	currentBranch, err := ctx.Branch()
	if err != nil {
		return err
	}
	currentUser, err := ctx.AuthLogin()
	if err != nil {
		return err
	}

	prPayload, err := api.PullRequests(apiClient, baseRepo, currentBranch, currentUser)
	if err != nil {
		return err
	}

	printHeader("Current branch")
	if prPayload.CurrentPR != nil {
		printPrs(*prPayload.CurrentPR)
	} else {
		message := fmt.Sprintf("  There is no pull request associated with %s", utils.Cyan("["+currentBranch+"]"))
		printMessage(message)
	}
	fmt.Println()

	printHeader("Created by you")
	if len(prPayload.ViewerCreated) > 0 {
		printPrs(prPayload.ViewerCreated...)
	} else {
		printMessage("  You have no open pull requests")
	}
	fmt.Println()

	printHeader("Requesting a code review from you")
	if len(prPayload.ReviewRequested) > 0 {
		printPrs(prPayload.ReviewRequested...)
	} else {
		printMessage("  You have no pull requests to review")
	}
	fmt.Println()

	return nil
}

func prList(cmd *cobra.Command, args []string) error {
	ctx := contextForCommand(cmd)
	apiClient, err := apiClientForContext(ctx)
	if err != nil {
		return err
	}

	baseRepo, err := ctx.BaseRepo()
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
	labels, err := cmd.Flags().GetStringArray("label")
	if err != nil {
		return err
	}

	var graphqlState []string
	switch state {
	case "open":
		graphqlState = []string{"OPEN"}
	case "closed":
		graphqlState = []string{"CLOSED"}
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

	prs, err := api.PullRequestList(apiClient, params, limit)
	if err != nil {
		return err
	}

	table := utils.NewTablePrinter(cmd.OutOrStdout())
	for _, pr := range prs {
		prNum := strconv.Itoa(pr.Number)
		if table.IsTTY() {
			prNum = "#" + prNum
		}
		table.AddField(prNum, nil, colorFuncForState(pr.State))
		table.AddField(pr.Title, nil, nil)
		table.AddField(pr.HeadLabel(), nil, utils.Cyan)
		table.EndRow()
	}
	err = table.Render()
	if err != nil {
		return err
	}

	return nil
}

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
	baseRepo, err := ctx.BaseRepo()
	if err != nil {
		return err
	}

	var openURL string
	if len(args) > 0 {
		if prNumber, err := strconv.Atoi(args[0]); err == nil {
			// TODO: move URL generation into GitHubRepository
			openURL = fmt.Sprintf("https://github.com/%s/%s/pull/%d", baseRepo.RepoOwner(), baseRepo.RepoName(), prNumber)
		} else {
			return fmt.Errorf("invalid pull request number: '%s'", args[0])
		}
	} else {
		apiClient, err := apiClientForContext(ctx)
		if err != nil {
			return err
		}
		currentBranch, err := ctx.Branch()
		if err != nil {
			return err
		}

		prs, err := api.PullRequestsForBranch(apiClient, baseRepo, currentBranch)
		if err != nil {
			return err
		} else if len(prs) < 1 {
			return fmt.Errorf("the '%s' branch has no open pull requests", currentBranch)
		}
		openURL = prs[0].URL
	}

	fmt.Printf("Opening %s in your browser.\n", openURL)
	return utils.OpenInBrowser(openURL)
}

func prCheckout(cmd *cobra.Command, args []string) error {
	prNumber, err := strconv.Atoi(args[0])
	if err != nil {
		return err
	}

	ctx := contextForCommand(cmd)
	currentBranch, _ := ctx.Branch()
	remotes, err := ctx.Remotes()
	if err != nil {
		return err
	}
	// FIXME: duplicates logic from fsContext.BaseRepo
	baseRemote, err := remotes.FindByName("upstream", "github", "origin", "*")
	if err != nil {
		return err
	}
	apiClient, err := apiClientForContext(ctx)
	if err != nil {
		return err
	}

	pr, err := api.PullRequestByNumber(apiClient, baseRemote, prNumber)
	if err != nil {
		return err
	}

	headRemote := baseRemote
	if pr.IsCrossRepository {
		headRemote, _ = remotes.FindByRepo(pr.HeadRepositoryOwner.Login, pr.HeadRepository.Name)
	}

	cmdQueue := [][]string{}

	newBranchName := pr.HeadRefName
	if headRemote != nil {
		// there is an existing git remote for PR head
		remoteBranch := fmt.Sprintf("%s/%s", headRemote.Name, pr.HeadRefName)
		refSpec := fmt.Sprintf("+refs/heads/%s:refs/remotes/%s", pr.HeadRefName, remoteBranch)

		cmdQueue = append(cmdQueue, []string{"git", "fetch", headRemote.Name, refSpec})

		// local branch already exists
		if git.VerifyRef("refs/heads/" + newBranchName) {
			cmdQueue = append(cmdQueue, []string{"git", "checkout", newBranchName})
			cmdQueue = append(cmdQueue, []string{"git", "merge", "--ff-only", fmt.Sprintf("refs/remotes/%s", remoteBranch)})
		} else {
			cmdQueue = append(cmdQueue, []string{"git", "checkout", "-b", newBranchName, "--no-track", remoteBranch})
			cmdQueue = append(cmdQueue, []string{"git", "config", fmt.Sprintf("branch.%s.remote", newBranchName), headRemote.Name})
			cmdQueue = append(cmdQueue, []string{"git", "config", fmt.Sprintf("branch.%s.merge", newBranchName), "refs/heads/" + pr.HeadRefName})
		}
	} else {
		// no git remote for PR head

		// avoid naming the new branch the same as the default branch
		if newBranchName == pr.HeadRepository.DefaultBranchRef.Name {
			newBranchName = fmt.Sprintf("%s/%s", pr.HeadRepositoryOwner.Login, newBranchName)
		}

		ref := fmt.Sprintf("refs/pull/%d/head", prNumber)
		if newBranchName == currentBranch {
			// PR head matches currently checked out branch
			cmdQueue = append(cmdQueue, []string{"git", "fetch", baseRemote.Name, ref})
			cmdQueue = append(cmdQueue, []string{"git", "merge", "--ff-only", "FETCH_HEAD"})
		} else {
			// create a new branch
			cmdQueue = append(cmdQueue, []string{"git", "fetch", baseRemote.Name, fmt.Sprintf("%s:%s", ref, newBranchName)})
			cmdQueue = append(cmdQueue, []string{"git", "checkout", newBranchName})
		}

		remote := baseRemote.Name
		mergeRef := ref
		if pr.MaintainerCanModify {
			remote = fmt.Sprintf("https://github.com/%s/%s.git", pr.HeadRepositoryOwner.Login, pr.HeadRepository.Name)
			mergeRef = fmt.Sprintf("refs/heads/%s", pr.HeadRefName)
		}
		if mc, err := git.Config(fmt.Sprintf("branch.%s.merge", newBranchName)); err != nil || mc == "" {
			cmdQueue = append(cmdQueue, []string{"git", "config", fmt.Sprintf("branch.%s.remote", newBranchName), remote})
			cmdQueue = append(cmdQueue, []string{"git", "config", fmt.Sprintf("branch.%s.merge", newBranchName), mergeRef})
		}
	}

	for _, args := range cmdQueue {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := utils.PrepareCmd(cmd).Run(); err != nil {
			return err
		}
	}

	return nil
}

func printPrs(prs ...api.PullRequest) {
	for _, pr := range prs {
		prNumber := fmt.Sprintf("#%d", pr.Number)
		fmt.Printf("  %s  %s %s", utils.Yellow(prNumber), truncate(50, pr.Title), utils.Cyan("["+pr.HeadLabel()+"]"))

		checks := pr.ChecksStatus()
		reviews := pr.ReviewStatus()
		if checks.Total > 0 || reviews.ChangesRequested || reviews.Approved {
			fmt.Printf("\n  ")
		}

		if checks.Total > 0 {
			var ratio string
			if checks.Failing > 0 {
				ratio = fmt.Sprintf("%d/%d", checks.Passing, checks.Total)
				ratio = utils.Red(ratio)
			} else if checks.Pending > 0 {
				ratio = fmt.Sprintf("%d/%d", checks.Passing, checks.Total)
				ratio = utils.Yellow(ratio)
			} else if checks.Passing == checks.Total {
				ratio = fmt.Sprintf("%d", checks.Total)
				ratio = utils.Green(ratio)
			}
			fmt.Printf(" - checks: %s", ratio)
		}

		if reviews.ChangesRequested {
			fmt.Printf(" - %s", utils.Red("changes requested"))
		} else if reviews.ReviewRequired {
			fmt.Printf(" - %s", utils.Yellow("review required"))
		} else if reviews.Approved {
			fmt.Printf(" - %s", utils.Green("approved"))
		}

		fmt.Printf("\n")
	}
}

func printHeader(s string) {
	fmt.Println(utils.Bold(s))
}

func printMessage(s string) {
	fmt.Println(utils.Gray(s))
}

func truncate(maxLength int, title string) string {
	if len(title) > maxLength {
		return title[0:maxLength-3] + "..."
	}
	return title
}
