package command

import (
	"fmt"
	"strconv"

	"github.com/github/gh-cli/api"
	"github.com/github/gh-cli/utils"
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(prCmd)
	prCmd.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "List pull requests",
			RunE:  prList,
		},
		&cobra.Command{
			Use:   "view [pr-number]",
			Short: "Open a pull request in the browser",
			RunE:  prView,
		},
	)
}

var prCmd = &cobra.Command{
	Use:   "pr",
	Short: "Work with pull requests",
	Long: `This command allows you to
work with pull requests.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("%+v is not a valid PR command", args)
	},
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
		fmt.Printf("  #%d %s %s\n", pr.Number, truncateTitle(pr.Title, 50), utils.Cyan("["+pr.HeadRefName+"]"))
	}
}

func printHeader(s string) {
	fmt.Println(utils.Bold(s))
}

func printMessage(s string) {
	fmt.Println(utils.Gray(s))
}

func truncateTitle(title string, maxLength int) string {
	if len(title) > maxLength {
		return title[0:maxLength-3] + "..."
	}
	return title
}
