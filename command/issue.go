package command

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/github/gh-cli/api"
	"github.com/github/gh-cli/utils"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(issueCmd)
	issueCmd.AddCommand(
		&cobra.Command{
			Use:   "status",
			Short: "Show status of relevant issues",
			RunE:  issueStatus,
		},
		&cobra.Command{
			Use:   "view <issue-number>",
			Args:  cobra.MinimumNArgs(1),
			Short: "View an issue in the browser",
			RunE:  issueView,
		},
	)
	issueCmd.AddCommand(issueCreateCmd)
	issueCreateCmd.Flags().StringP("title", "t", "",
		"Supply a title. Will prompt for one otherwise.")
	issueCreateCmd.Flags().StringP("body", "b", "",
		"Supply a body. Will prompt for one otherwise.")
	issueCreateCmd.Flags().BoolP("web", "w", false, "open the web browser to create an issue")

	issueListCmd := &cobra.Command{
		Use:   "list",
		Short: "List open issues",
		RunE:  issueList,
	}
	issueListCmd.Flags().StringP("assignee", "a", "", "filter by assignee")
	issueListCmd.Flags().StringSliceP("label", "l", nil, "filter by label")
	issueListCmd.Flags().StringP("state", "s", "", "filter by state (open|closed|all)")
	issueListCmd.Flags().IntP("limit", "L", 30, "maximum number of issues to fetch")
	issueCmd.AddCommand((issueListCmd))
}

var issueCmd = &cobra.Command{
	Use:   "issue",
	Short: "Work with GitHub issues",
	Long:  `Helps you work with issues.`,
}
var issueCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new issue",
	RunE:  issueCreate,
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

	state, err := cmd.Flags().GetString("state")
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

	limit, err := cmd.Flags().GetInt("limit")
	if err != nil {
		return err
	}

	issues, err := api.IssueList(apiClient, baseRepo, state, labels, assignee, limit)
	if err != nil {
		return err
	}

	if len(issues) > 0 {
		printIssues("", issues...)
	} else {
		message := fmt.Sprintf("There are no open issues")
		printMessage(message)
	}
	return nil
}

func issueStatus(cmd *cobra.Command, args []string) error {
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

	issuePayload, err := api.IssueStatus(apiClient, baseRepo, currentUser)
	if err != nil {
		return err
	}

	printHeader("Issues assigned to you")
	if issuePayload.Assigned != nil {
		printIssues("  ", issuePayload.Assigned...)
	} else {
		message := fmt.Sprintf("  There are no issues assgined to you")
		printMessage(message)
	}
	fmt.Println()

	printHeader("Issues mentioning you")
	if len(issuePayload.Mentioned) > 0 {
		printIssues("  ", issuePayload.Mentioned...)
	} else {
		printMessage("  There are no issues mentioning you")
	}
	fmt.Println()

	printHeader("Recent issues")
	if len(issuePayload.Recent) > 0 {
		printIssues("  ", issuePayload.Recent...)
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

func issueCreate(cmd *cobra.Command, args []string) error {
	ctx := contextForCommand(cmd)

	baseRepo, err := ctx.BaseRepo()
	if err != nil {
		return err
	}

	if isWeb, err := cmd.Flags().GetBool("web"); err == nil && isWeb {
		// TODO: move URL generation into GitHubRepository
		openURL := fmt.Sprintf("https://github.com/%s/%s/issues/new", baseRepo.RepoOwner(), baseRepo.RepoName())
		// TODO: figure out how to stub this in tests
		if stat, err := os.Stat(".github/ISSUE_TEMPLATE"); err == nil && stat.IsDir() {
			openURL += "/choose"
		}
		return utils.OpenInBrowser(openURL)
	}

	apiClient, err := apiClientForContext(ctx)
	if err != nil {
		return err
	}

	title, err := cmd.Flags().GetString("title")
	if err != nil {
		return errors.Wrap(err, "could not parse title")
	}
	body, err := cmd.Flags().GetString("body")
	if err != nil {
		return errors.Wrap(err, "could not parse body")
	}

	interactive := title == "" || body == ""

	if interactive {
		tb, err := titleBodySurvey(cmd, title, body)
		if err != nil {
			return errors.Wrap(err, "could not collect title and/or body")
		}

		if tb == nil {
			// editing was canceled, we can just leave
			return nil
		}

		if title == "" {
			title = tb.Title
		}
		if body == "" {
			body = tb.Body
		}
	}
	params := map[string]interface{}{
		"title": title,
		"body":  body,
	}

	newIssue, err := api.IssueCreate(apiClient, baseRepo, params)
	if err != nil {
		return err
	}

	fmt.Fprintln(cmd.OutOrStdout(), newIssue.URL)
	return nil
}

func printIssues(prefix string, issues ...api.Issue) {
	for _, issue := range issues {
		number := utils.Green("#" + strconv.Itoa(issue.Number))
		var coloredLabels string
		if len(issue.Labels) > 0 {
			var ellipse string
			if issue.TotalLabelCount > len(issue.Labels) {
				ellipse = "…"
			}
			coloredLabels = utils.Gray(fmt.Sprintf(" (%s%s)", strings.Join(issue.Labels, ", "), ellipse))
		}
		fmt.Printf("%s%s %s %s\n", prefix, number, truncate(70, issue.Title), coloredLabels)
	}
}
