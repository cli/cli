package command

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/github/gh-cli/api"
	"github.com/github/gh-cli/git"
	"github.com/logrusorgru/aurora"
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
			Use:   "view",
			Short: "Open the pull request in the browser",
			RunE:  prShow,
		},
	)
}

var prCmd = &cobra.Command{
	Use:   "pr",
	Short: "Work with pull requests",
	Long: `This command allows you to
work with pull requests.`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("pr")
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
		message := fmt.Sprintf("  There is no pull request associated with %s", aurora.Cyan("["+currentBranch()+"]"))
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

func show(cmd *cobra.Command, args []string) error {
	project := project()

	var openURL string
	if len(number) > 0 {
		if prNumber, err := strconv.Atoi(number[0]); err == nil {
			openURL = project.WebURL("", "", fmt.Sprintf("pull/%d", prNumber))
		} else {
			return fmt.Errorf("invalid pull request number: '%s'", number[0])
		}
	} else {
		pr, err := pullRequestForCurrentBranch()
		if err != nil {
			return err
		}
		openURL = pr.HtmlUrl
	}

	return openInBrowser(openURL)
}

func printPrs(prs ...api.PullRequest) {
	for _, pr := range prs {
		fmt.Printf("  #%d %s [%s]\n", pr.Number, truncateTitle(pr.Title), aurora.Cyan(pr.HeadRefName))
	}
}

func printHeader(s string) {
	fmt.Println(aurora.Bold(s))
}

func printMessage(s string) {
	fmt.Println(aurora.Gray(8, s))
}

func truncateTitle(title string) string {
	const maxLength = 50

	if len(title) > maxLength {
		return title[0:maxLength-3] + "..."
	}
	return title
}

func currentBranch() string {
	currentBranch, err := git.Head()
	if err != nil {
		panic(err)
	}

	return strings.Replace(currentBranch, "refs/heads/", "", 1)
}
