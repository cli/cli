package command

import (
	"fmt"
	"strconv"

	"github.com/github/gh-cli/api"
	"github.com/github/gh-cli/context"
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
	prPayload, err := api.PullRequests()
	if err != nil {
		return err
	}

	printHeader("Current branch")
	if prPayload.CurrentPR != nil {
		printPrs(*prPayload.CurrentPR)
	} else {
		currentBranch, err := context.Current().Branch()
		if err != nil {
			return err
		}
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
	baseRepo, err := context.Current().BaseRepo()
	if err != nil {
		return err
	}

	var openURL string
	if len(args) > 0 {
		if prNumber, err := strconv.Atoi(args[0]); err == nil {
			// TODO: move URL generation into GitHubRepository
			openURL = fmt.Sprintf("https://github.com/%s/%s/pull/%d", baseRepo.Owner, baseRepo.Name, prNumber)
		} else {
			return fmt.Errorf("invalid pull request number: '%s'", args[0])
		}
	} else {
		prPayload, err := api.PullRequests()
		if err != nil {
			return err
		} else if prPayload.CurrentPR == nil {
			branch, err := context.Current().Branch()
			if err != nil {
				return err
			}
			fmt.Printf("The [%s] branch has no open PRs", branch)
			return nil
		}
		openURL = prPayload.CurrentPR.URL
	}

	fmt.Printf("Opening %s in your browser.\n", openURL)
	return utils.OpenInBrowser(openURL)
}

func printPrs(prs ...api.PullRequest) {
	for _, pr := range prs {
		fmt.Printf("  #%d %s %s\n", pr.Number, truncateTitle(pr.Title), utils.Cyan("["+pr.HeadRefName+"]"))
	}
}

func printHeader(s string) {
	fmt.Println(utils.Bold(s))
}

func printMessage(s string) {
	fmt.Println(utils.Gray(s))
}

func truncateTitle(title string) string {
	const maxLength = 50

	if len(title) > maxLength {
		return title[0:maxLength-3] + "..."
	}
	return title
}
