package command

import (
	"fmt"
	"strconv"

	"github.com/github/gh-cli/api"
	"github.com/github/gh-cli/utils"
	"github.com/spf13/cobra"
)

func init() {
	var issueCmd = &cobra.Command{
		Use:   "issue",
		Short: "Work with GitHub issues",
		Long:  `This command allows you to work with issues.`,
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("%+v is not a valid issue command", args)
		},
	}

	issueCmd.AddCommand(
		&cobra.Command{
			Use:   "status",
			Short: "Display issue status",
			RunE:  issueList,
		},
		&cobra.Command{
			Use:   "view [issue-number]",
			Args:  cobra.MinimumNArgs(1),
			Short: "Open a issue in the browser",
			RunE:  issueView,
		},
	)

	RootCmd.AddCommand(issueCmd)
}

func issueList(cmd *cobra.Command, args []string) error {
	ctx := contextForCommand(cmd)
	apiClient, err := apiClientForContext(ctx)
	if err != nil {
		return err
	}

	baseRepo, err := ctx.BaseRepo()
	if err != nil {
		return err
	}

	currentUser, err := ctx.AuthLogin()
	if err != nil {
		return err
	}

	issuePayload, err := api.Issues(apiClient, baseRepo, currentUser)
	if err != nil {
		return err
	}

	printHeader("Issues assigned to you")
	if issuePayload.Assigned != nil {
		printIssues(issuePayload.Assigned...)
	} else {
		message := fmt.Sprintf("  There are no issues assgined to you")
		printMessage(message)
	}
	fmt.Println()

	printHeader("Issues mentioning you")
	if len(issuePayload.Mentioned) > 0 {
		printIssues(issuePayload.Mentioned...)
	} else {
		printMessage("  There are no issues mentioning you")
	}
	fmt.Println()

	printHeader("Recent issues")
	if len(issuePayload.Recent) > 0 {
		printIssues(issuePayload.Recent...)
	} else {
		printMessage("  There are no recent issues")
	}
	fmt.Println()

	return nil
}

func issueView(cmd *cobra.Command, args []string) error {
	ctx := contextForCommand(cmd)

	baseRepo, err := ctx.BaseRepo()
	if err != nil {
		return err
	}

	var openURL string
	if number, err := strconv.Atoi(args[0]); err == nil {
		// TODO: move URL generation into GitHubRepository
		openURL = fmt.Sprintf("https://github.com/%s/%s/issues/%d", baseRepo.RepoOwner(), baseRepo.RepoName(), number)
	} else {
		return fmt.Errorf("invalid issue number: '%s'", args[0])
	}

	fmt.Printf("Opening %s in your browser.\n", openURL)
	return utils.OpenInBrowser(openURL)
}

func printIssues(issues ...api.Issue) {
	for _, issue := range issues {
		fmt.Printf("  #%d %s\n", issue.Number, truncate(70, issue.Title))
	}
}
