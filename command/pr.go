package command

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/github/gh-cli/api"
	"github.com/github/gh-cli/git"
	"github.com/github/gh-cli/github"
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
		message := fmt.Sprintf("  There is no pull request associated with %s", utils.Cyan("["+currentBranch()+"]"))
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
	project := project()

	var openURL string
	if len(args) > 0 {
		if prNumber, err := strconv.Atoi(args[0]); err == nil {
			openURL = project.WebURL("", "", fmt.Sprintf("pull/%d", prNumber))
		} else {
			return fmt.Errorf("invalid pull request number: '%s'", args[0])
		}
	} else {
		prPayload, err := api.PullRequests()
		if err != nil || prPayload.CurrentPR == nil {
			branch := currentBranch()
			return fmt.Errorf("The [%s] branch has no open PRs", branch)
		}
		openURL = prPayload.CurrentPR.URL
	}

	fmt.Printf("Opening %s in your browser.\n", openURL)
	return openInBrowser(openURL)
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

func openInBrowser(url string) error {
	launcher, err := utils.BrowserLauncher()
	if err != nil {
		return err
	}
	endingArgs := append(launcher[1:], url)
	return exec.Command(launcher[0], endingArgs...).Run()
}

// The functions below should be replaced at some point by the context package
// 🧨🧨🧨🧨🧨🧨🧨🧨🧨🧨🧨🧨🧨🧨🧨🧨🧨🧨🧨🧨🧨🧨🧨🧨🧨🧨🧨🧨🧨🧨🧨🧨🧨🧨
func currentBranch() string {
	currentBranch, err := git.Head()
	if err != nil {
		panic(err)
	}

	return strings.Replace(currentBranch, "refs/heads/", "", 1)
}

func project() github.Project {
	if repoFromEnv := os.Getenv("GH_REPO"); repoFromEnv != "" {
		repoURL, err := url.Parse(fmt.Sprintf("https://github.com/%s.git", repoFromEnv))
		if err != nil {
			panic(err)
		}
		project, err := github.NewProjectFromURL(repoURL)
		if err != nil {
			panic(err)
		}
		return *project
	}

	remotes, err := github.Remotes()
	if err != nil {
		panic(err)
	}

	for _, remote := range remotes {
		if project, err := remote.Project(); err == nil {
			return *project
		}
	}

	panic("Could not get the project. What is a project? I don't know, it's kind of like a git repository I think?")
}
