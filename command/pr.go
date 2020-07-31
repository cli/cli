package command

import (
	"fmt"
	"strconv"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmd/pr/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/text"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func init() {
	prCmd.PersistentFlags().StringP("repo", "R", "", "Select another repository using the `OWNER/REPO` format")

	RootCmd.AddCommand(prCmd)
	prCmd.AddCommand(prCreateCmd)
	prCmd.AddCommand(prCloseCmd)
	prCmd.AddCommand(prReopenCmd)
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
var prReadyCmd = &cobra.Command{
	Use:   "ready [<number> | <url> | <branch>]",
	Short: "Mark a pull request as ready for review",
	Args:  cobra.MaximumNArgs(1),
	RunE:  prReady,
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
		table.AddField(text.ReplaceExcessiveWhitespace(pr.Title), nil, nil)
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
