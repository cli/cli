package command

import (
	"fmt"
	"os"
	"strconv"

	"github.com/github/gh-cli/api"
	"github.com/github/gh-cli/utils"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh/terminal"
)

func init() {
	RootCmd.AddCommand(prCmd)
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
	Long:  `Helps you work with pull requests.`,
}
var prListCmd = &cobra.Command{
	Use:   "list",
	Short: "List pull requests",
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

	tty := false
	ttyWidth := 80
	out := cmd.OutOrStdout()
	if outFile, isFile := out.(*os.File); isFile {
		fd := int(outFile.Fd())
		tty = terminal.IsTerminal(fd)
		if w, _, err := terminal.GetSize(fd); err == nil {
			ttyWidth = w
		}
	}

	numWidth := 0
	maxTitleWidth := 0
	for _, pr := range prs {
		numLen := len(strconv.Itoa(pr.Number)) + 1
		if numLen > numWidth {
			numWidth = numLen
		}
		if len(pr.Title) > maxTitleWidth {
			maxTitleWidth = len(pr.Title)
		}
	}

	branchWidth := 40
	titleWidth := ttyWidth - branchWidth - 2 - numWidth - 2

	if maxTitleWidth < titleWidth {
		branchWidth += titleWidth - maxTitleWidth
		titleWidth = maxTitleWidth
	}

	for _, pr := range prs {
		if tty {
			prNum := fmt.Sprintf("% *s", numWidth, fmt.Sprintf("#%d", pr.Number))
			switch pr.State {
			case "OPEN":
				prNum = utils.Green(prNum)
			case "CLOSED":
				prNum = utils.Red(prNum)
			case "MERGED":
				prNum = utils.Magenta(prNum)
			}
			prBranch := utils.Cyan(truncate(branchWidth, pr.HeadRefName))
			fmt.Fprintf(out, "%s  %-*s  %s\n", prNum, titleWidth, truncate(titleWidth, pr.Title), prBranch)
		} else {
			fmt.Fprintf(out, "%d\t%s\t%s\n", pr.Number, pr.Title, pr.HeadRefName)
		}
	}
	return nil
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

func printPrs(prs ...api.PullRequest) {
	for _, pr := range prs {
		prNumber := fmt.Sprintf("#%d", pr.Number)
		fmt.Printf("  %s  %s %s", utils.Yellow(prNumber), truncate(50, pr.Title), utils.Cyan("["+pr.HeadRefName+"]"))

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
