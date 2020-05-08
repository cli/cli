package command

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cli/cli/api"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/githubtemplate"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func init() {
	RootCmd.AddCommand(issueCmd)
	issueCmd.AddCommand(issueStatusCmd)

	issueCmd.AddCommand(issueCreateCmd)
	issueCreateCmd.Flags().StringP("title", "t", "",
		"Supply a title. Will prompt for one otherwise.")
	issueCreateCmd.Flags().StringP("body", "b", "",
		"Supply a body. Will prompt for one otherwise.")
	issueCreateCmd.Flags().BoolP("web", "w", false, "Open the browser to create an issue")
	issueCreateCmd.Flags().StringSliceP("assignee", "a", nil, "Assign a person by their `login`")
	issueCreateCmd.Flags().StringSliceP("label", "l", nil, "Add a label by `name`")
	issueCreateCmd.Flags().StringSliceP("project", "p", nil, "Add the issue to a project by `name`")
	issueCreateCmd.Flags().StringP("milestone", "m", "", "Add the issue to a milestone by `name`")

	issueCmd.AddCommand(issueListCmd)
	issueListCmd.Flags().StringP("assignee", "a", "", "Filter by assignee")
	issueListCmd.Flags().StringSliceP("label", "l", nil, "Filter by label")
	issueListCmd.Flags().StringP("state", "s", "open", "Filter by state: {open|closed|all}")
	issueListCmd.Flags().IntP("limit", "L", 30, "Maximum number of issues to fetch")
	issueListCmd.Flags().StringP("author", "A", "", "Filter by author")

	issueCmd.AddCommand(issueViewCmd)
	issueViewCmd.Flags().BoolP("web", "w", false, "Open an issue in the browser")

	issueCmd.AddCommand(issueCloseCmd)
	issueCmd.AddCommand(issueReopenCmd)
}

var issueCmd = &cobra.Command{
	Use:   "issue",
	Short: "Create and view issues",
	Long: `Work with GitHub issues.

An issue can be supplied as argument in any of the following formats:
- by number, e.g. "123"; or
- by URL, e.g. "https://github.com/OWNER/REPO/issues/123".`,
}
var issueCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new issue",
	RunE:  issueCreate,
}
var issueListCmd = &cobra.Command{
	Use:   "list",
	Short: "List and filter issues in this repository",
	RunE:  issueList,
}
var issueStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show status of relevant issues",
	RunE:  issueStatus,
}
var issueViewCmd = &cobra.Command{
	Use: "view {<number> | <url>}",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return FlagError{errors.New("issue number or URL required as argument")}
		}
		return nil
	},
	Short: "View an issue",
	Long: `Display the title, body, and other information about an issue.

With '--web', open the issue in a web browser instead.`,
	RunE: issueView,
}
var issueCloseCmd = &cobra.Command{
	Use:   "close {<number> | <url>}",
	Short: "close issue",
	Args:  cobra.ExactArgs(1),
	RunE:  issueClose,
}
var issueReopenCmd = &cobra.Command{
	Use:   "reopen {<number> | <url>}",
	Short: "reopen issue",
	Args:  cobra.ExactArgs(1),
	RunE:  issueReopen,
}

func issueList(cmd *cobra.Command, args []string) error {
	ctx := contextForCommand(cmd)
	apiClient, err := apiClientForContext(ctx)
	if err != nil {
		return err
	}

	baseRepo, err := determineBaseRepo(cmd, ctx)
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

	author, err := cmd.Flags().GetString("author")
	if err != nil {
		return err
	}

	listResult, err := api.IssueList(apiClient, baseRepo, state, labels, assignee, limit, author)
	if err != nil {
		return err
	}

	hasFilters := false
	cmd.Flags().Visit(func(f *pflag.Flag) {
		switch f.Name {
		case "state", "label", "assignee", "author":
			hasFilters = true
		}
	})

	title := listHeader(ghrepo.FullName(baseRepo), "issue", len(listResult.Issues), listResult.TotalCount, hasFilters)
	// TODO: avoid printing header if piped to a script
	fmt.Fprintf(colorableErr(cmd), "\n%s\n\n", title)

	out := cmd.OutOrStdout()

	printIssues(out, "", len(listResult.Issues), listResult.Issues)

	return nil
}

func issueStatus(cmd *cobra.Command, args []string) error {
	ctx := contextForCommand(cmd)
	apiClient, err := apiClientForContext(ctx)
	if err != nil {
		return err
	}

	baseRepo, err := determineBaseRepo(cmd, ctx)
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

	out := colorableOut(cmd)

	fmt.Fprintln(out, "")
	fmt.Fprintf(out, "Relevant issues in %s\n", ghrepo.FullName(baseRepo))
	fmt.Fprintln(out, "")

	printHeader(out, "Issues assigned to you")
	if issuePayload.Assigned.TotalCount > 0 {
		printIssues(out, "  ", issuePayload.Assigned.TotalCount, issuePayload.Assigned.Issues)
	} else {
		message := "  There are no issues assigned to you"
		printMessage(out, message)
	}
	fmt.Fprintln(out)

	printHeader(out, "Issues mentioning you")
	if issuePayload.Mentioned.TotalCount > 0 {
		printIssues(out, "  ", issuePayload.Mentioned.TotalCount, issuePayload.Mentioned.Issues)
	} else {
		printMessage(out, "  There are no issues mentioning you")
	}
	fmt.Fprintln(out)

	printHeader(out, "Issues opened by you")
	if issuePayload.Authored.TotalCount > 0 {
		printIssues(out, "  ", issuePayload.Authored.TotalCount, issuePayload.Authored.Issues)
	} else {
		printMessage(out, "  There are no issues opened by you")
	}
	fmt.Fprintln(out)

	return nil
}

func issueView(cmd *cobra.Command, args []string) error {
	ctx := contextForCommand(cmd)

	apiClient, err := apiClientForContext(ctx)
	if err != nil {
		return err
	}

	baseRepo, err := determineBaseRepo(cmd, ctx)
	if err != nil {
		return err
	}

	issue, err := issueFromArg(apiClient, baseRepo, args[0])
	if err != nil {
		return err
	}
	openURL := issue.URL

	web, err := cmd.Flags().GetBool("web")
	if err != nil {
		return err
	}

	if web {
		fmt.Fprintf(cmd.ErrOrStderr(), "Opening %s in your browser.\n", openURL)
		return utils.OpenInBrowser(openURL)
	} else {
		out := colorableOut(cmd)
		return printIssuePreview(out, issue)
	}

}

func issueStateTitleWithColor(state string) string {
	colorFunc := colorFuncForState(state)
	return colorFunc(strings.Title(strings.ToLower(state)))
}

func listHeader(repoName string, itemName string, matchCount int, totalMatchCount int, hasFilters bool) string {
	if totalMatchCount == 0 {
		if hasFilters {
			return fmt.Sprintf("No %ss match your search in %s", itemName, repoName)
		}
		return fmt.Sprintf("There are no open %ss in %s", itemName, repoName)
	}

	if hasFilters {
		matchVerb := "match"
		if totalMatchCount == 1 {
			matchVerb = "matches"
		}
		return fmt.Sprintf("Showing %d of %s in %s that %s your search", matchCount, utils.Pluralize(totalMatchCount, itemName), repoName, matchVerb)
	}

	return fmt.Sprintf("Showing %d of %s in %s", matchCount, utils.Pluralize(totalMatchCount, itemName), repoName)
}

func printIssuePreview(out io.Writer, issue *api.Issue) error {
	now := time.Now()
	ago := now.Sub(issue.CreatedAt)

	// Header (Title and State)
	fmt.Fprintln(out, utils.Bold(issue.Title))
	fmt.Fprint(out, issueStateTitleWithColor(issue.State))
	fmt.Fprintln(out, utils.Gray(fmt.Sprintf(
		" • %s opened %s • %s",
		issue.Author.Login,
		utils.FuzzyAgo(ago),
		utils.Pluralize(issue.Comments.TotalCount, "comment"),
	)))

	// Metadata
	fmt.Fprintln(out)
	if assignees := issueAssigneeList(*issue); assignees != "" {
		fmt.Fprint(out, utils.Bold("Assignees: "))
		fmt.Fprintln(out, assignees)
	}
	if labels := issueLabelList(*issue); labels != "" {
		fmt.Fprint(out, utils.Bold("Labels: "))
		fmt.Fprintln(out, labels)
	}
	if projects := issueProjectList(*issue); projects != "" {
		fmt.Fprint(out, utils.Bold("Projects: "))
		fmt.Fprintln(out, projects)
	}
	if issue.Milestone.Title != "" {
		fmt.Fprint(out, utils.Bold("Milestone: "))
		fmt.Fprintln(out, issue.Milestone.Title)
	}

	// Body
	if issue.Body != "" {
		fmt.Fprintln(out)
		md, err := utils.RenderMarkdown(issue.Body)
		if err != nil {
			return err
		}
		fmt.Fprintln(out, md)
	}
	fmt.Fprintln(out)

	// Footer
	fmt.Fprintf(out, utils.Gray("View this issue on GitHub: %s\n"), issue.URL)
	return nil
}

var issueURLRE = regexp.MustCompile(`^https://github\.com/([^/]+)/([^/]+)/issues/(\d+)`)

func issueFromArg(apiClient *api.Client, baseRepo ghrepo.Interface, arg string) (*api.Issue, error) {
	if issueNumber, err := strconv.Atoi(strings.TrimPrefix(arg, "#")); err == nil {
		return api.IssueByNumber(apiClient, baseRepo, issueNumber)
	}

	if m := issueURLRE.FindStringSubmatch(arg); m != nil {
		issueNumber, _ := strconv.Atoi(m[3])
		return api.IssueByNumber(apiClient, baseRepo, issueNumber)
	}

	return nil, fmt.Errorf("invalid issue format: %q", arg)
}

func issueCreate(cmd *cobra.Command, args []string) error {
	ctx := contextForCommand(cmd)

	// NB no auto forking like over in pr create
	baseRepo, err := determineBaseRepo(cmd, ctx)
	if err != nil {
		return err
	}

	baseOverride, err := cmd.Flags().GetString("repo")
	if err != nil {
		return err
	}

	var templateFiles []string
	if baseOverride == "" {
		if rootDir, err := git.ToplevelDir(); err == nil {
			// TODO: figure out how to stub this in tests
			templateFiles = githubtemplate.Find(rootDir, "ISSUE_TEMPLATE")
		}
	}

	title, err := cmd.Flags().GetString("title")
	if err != nil {
		return fmt.Errorf("could not parse title: %w", err)
	}
	body, err := cmd.Flags().GetString("body")
	if err != nil {
		return fmt.Errorf("could not parse body: %w", err)
	}

	assignees, err := cmd.Flags().GetStringSlice("assignee")
	if err != nil {
		return fmt.Errorf("could not parse assignees: %w", err)
	}
	labelNames, err := cmd.Flags().GetStringSlice("label")
	if err != nil {
		return fmt.Errorf("could not parse labels: %w", err)
	}
	projectNames, err := cmd.Flags().GetStringSlice("project")
	if err != nil {
		return fmt.Errorf("could not parse projects: %w", err)
	}
	milestoneTitle, err := cmd.Flags().GetString("milestone")
	if err != nil {
		return fmt.Errorf("could not parse milestone: %w", err)
	}

	if isWeb, err := cmd.Flags().GetBool("web"); err == nil && isWeb {
		// TODO: move URL generation into GitHubRepository
		openURL := fmt.Sprintf("https://github.com/%s/issues/new", ghrepo.FullName(baseRepo))
		if title != "" || body != "" {
			openURL += fmt.Sprintf(
				"?title=%s&body=%s",
				url.QueryEscape(title),
				url.QueryEscape(body),
			)
		} else if len(templateFiles) > 1 {
			openURL += "/choose"
		}
		cmd.Printf("Opening %s in your browser.\n", displayURL(openURL))
		return utils.OpenInBrowser(openURL)
	}

	fmt.Fprintf(colorableErr(cmd), "\nCreating issue in %s\n\n", ghrepo.FullName(baseRepo))

	apiClient, err := apiClientForContext(ctx)
	if err != nil {
		return err
	}

	repo, err := api.GitHubRepo(apiClient, baseRepo)
	if err != nil {
		return err
	}
	if !repo.HasIssuesEnabled {
		return fmt.Errorf("the '%s' repository has disabled issues", ghrepo.FullName(baseRepo))
	}

	action := SubmitAction
	tb := issueMetadataState{
		Assignees: assignees,
		Labels:    labelNames,
		Projects:  projectNames,
		Milestone: milestoneTitle,
	}

	interactive := !(cmd.Flags().Changed("title") && cmd.Flags().Changed("body"))

	if interactive {
		err := titleBodySurvey(cmd, &tb, apiClient, baseRepo, title, body, defaults{}, templateFiles, false, repo.ViewerCanTriage())
		if err != nil {
			return fmt.Errorf("could not collect title and/or body: %w", err)
		}

		action = tb.Action

		if tb.Action == CancelAction {
			fmt.Fprintln(cmd.ErrOrStderr(), "Discarding.")

			return nil
		}

		if title == "" {
			title = tb.Title
		}
		if body == "" {
			body = tb.Body
		}
	} else {
		if title == "" {
			return fmt.Errorf("title can't be blank")
		}
	}

	if action == PreviewAction {
		openURL := fmt.Sprintf(
			"https://github.com/%s/issues/new/?title=%s&body=%s",
			ghrepo.FullName(baseRepo),
			url.QueryEscape(title),
			url.QueryEscape(body),
		)
		// TODO could exceed max url length for explorer
		fmt.Fprintf(cmd.ErrOrStderr(), "Opening %s in your browser.\n", displayURL(openURL))
		return utils.OpenInBrowser(openURL)
	} else if action == SubmitAction {
		params := map[string]interface{}{
			"title": title,
			"body":  body,
		}

		if tb.HasMetadata() {
			if tb.MetadataResult == nil {
				metadataInput := api.RepoMetadataInput{
					Assignees:  len(tb.Assignees) > 0,
					Labels:     len(tb.Labels) > 0,
					Projects:   len(tb.Projects) > 0,
					Milestones: tb.Milestone != "",
				}

				// TODO: for non-interactive mode, only translate given objects to GraphQL IDs
				tb.MetadataResult, err = api.RepoMetadata(apiClient, baseRepo, metadataInput)
				if err != nil {
					return err
				}
			}

			err = addMetadataToIssueParams(params, tb.MetadataResult, tb.Assignees, tb.Labels, tb.Projects, tb.Milestone)
			if err != nil {
				return err
			}
		}

		newIssue, err := api.IssueCreate(apiClient, repo, params)
		if err != nil {
			return err
		}

		fmt.Fprintln(cmd.OutOrStdout(), newIssue.URL)
	} else {
		panic("Unreachable state")
	}

	return nil
}

func addMetadataToIssueParams(params map[string]interface{}, metadata *api.RepoMetadataResult, assignees, labelNames, projectNames []string, milestoneTitle string) error {
	assigneeIDs, err := metadata.MembersToIDs(assignees)
	if err != nil {
		return fmt.Errorf("could not assign user: %w", err)
	}
	params["assigneeIds"] = assigneeIDs

	labelIDs, err := metadata.LabelsToIDs(labelNames)
	if err != nil {
		return fmt.Errorf("could not add label: %w", err)
	}
	params["labelIds"] = labelIDs

	projectIDs, err := metadata.ProjectsToIDs(projectNames)
	if err != nil {
		return fmt.Errorf("could not add to project: %w", err)
	}
	params["projectIds"] = projectIDs

	if milestoneTitle != "" {
		milestoneID, err := metadata.MilestoneToID(milestoneTitle)
		if err != nil {
			return fmt.Errorf("could not add to milestone '%s': %w", milestoneTitle, err)
		}
		params["milestoneId"] = milestoneID
	}

	return nil
}

func printIssues(w io.Writer, prefix string, totalCount int, issues []api.Issue) {
	table := utils.NewTablePrinter(w)
	for _, issue := range issues {
		issueNum := strconv.Itoa(issue.Number)
		if table.IsTTY() {
			issueNum = "#" + issueNum
		}
		issueNum = prefix + issueNum
		labels := issueLabelList(issue)
		if labels != "" && table.IsTTY() {
			labels = fmt.Sprintf("(%s)", labels)
		}
		now := time.Now()
		ago := now.Sub(issue.UpdatedAt)
		table.AddField(issueNum, nil, colorFuncForState(issue.State))
		table.AddField(replaceExcessiveWhitespace(issue.Title), nil, nil)
		table.AddField(labels, nil, utils.Gray)
		table.AddField(utils.FuzzyAgo(ago), nil, utils.Gray)
		table.EndRow()
	}
	_ = table.Render()
	remaining := totalCount - len(issues)
	if remaining > 0 {
		fmt.Fprintf(w, utils.Gray("%sAnd %d more\n"), prefix, remaining)
	}
}

func issueAssigneeList(issue api.Issue) string {
	if len(issue.Assignees.Nodes) == 0 {
		return ""
	}

	AssigneeNames := make([]string, 0, len(issue.Assignees.Nodes))
	for _, assignee := range issue.Assignees.Nodes {
		AssigneeNames = append(AssigneeNames, assignee.Login)
	}

	list := strings.Join(AssigneeNames, ", ")
	if issue.Assignees.TotalCount > len(issue.Assignees.Nodes) {
		list += ", …"
	}
	return list
}

func issueLabelList(issue api.Issue) string {
	if len(issue.Labels.Nodes) == 0 {
		return ""
	}

	labelNames := make([]string, 0, len(issue.Labels.Nodes))
	for _, label := range issue.Labels.Nodes {
		labelNames = append(labelNames, label.Name)
	}

	list := strings.Join(labelNames, ", ")
	if issue.Labels.TotalCount > len(issue.Labels.Nodes) {
		list += ", …"
	}
	return list
}

func issueProjectList(issue api.Issue) string {
	if len(issue.ProjectCards.Nodes) == 0 {
		return ""
	}

	projectNames := make([]string, 0, len(issue.ProjectCards.Nodes))
	for _, project := range issue.ProjectCards.Nodes {
		colName := project.Column.Name
		if colName == "" {
			colName = "Awaiting triage"
		}
		projectNames = append(projectNames, fmt.Sprintf("%s (%s)", project.Project.Name, colName))
	}

	list := strings.Join(projectNames, ", ")
	if issue.ProjectCards.TotalCount > len(issue.ProjectCards.Nodes) {
		list += ", …"
	}
	return list
}

func issueClose(cmd *cobra.Command, args []string) error {
	ctx := contextForCommand(cmd)
	apiClient, err := apiClientForContext(ctx)
	if err != nil {
		return err
	}

	baseRepo, err := determineBaseRepo(cmd, ctx)
	if err != nil {
		return err
	}

	issue, err := issueFromArg(apiClient, baseRepo, args[0])
	var idErr *api.IssuesDisabledError
	if errors.As(err, &idErr) {
		return fmt.Errorf("issues disabled for %s", ghrepo.FullName(baseRepo))
	} else if err != nil {
		return fmt.Errorf("failed to find issue #%d: %w", issue.Number, err)
	}

	if issue.Closed {
		fmt.Fprintf(colorableErr(cmd), "%s Issue #%d is already closed\n", utils.Yellow("!"), issue.Number)
		return nil
	}

	err = api.IssueClose(apiClient, baseRepo, *issue)
	if err != nil {
		return fmt.Errorf("API call failed:%w", err)
	}

	fmt.Fprintf(colorableErr(cmd), "%s Closed issue #%d\n", utils.Red("✔"), issue.Number)

	return nil
}

func issueReopen(cmd *cobra.Command, args []string) error {
	ctx := contextForCommand(cmd)
	apiClient, err := apiClientForContext(ctx)
	if err != nil {
		return err
	}

	baseRepo, err := determineBaseRepo(cmd, ctx)
	if err != nil {
		return err
	}

	issue, err := issueFromArg(apiClient, baseRepo, args[0])
	var idErr *api.IssuesDisabledError
	if errors.As(err, &idErr) {
		return fmt.Errorf("issues disabled for %s", ghrepo.FullName(baseRepo))
	} else if err != nil {
		return fmt.Errorf("failed to find issue #%d: %w", issue.Number, err)
	}

	if !issue.Closed {
		fmt.Fprintf(colorableErr(cmd), "%s Issue #%d is already open\n", utils.Yellow("!"), issue.Number)
		return nil
	}

	err = api.IssueReopen(apiClient, baseRepo, *issue)
	if err != nil {
		return fmt.Errorf("API call failed:%w", err)
	}

	fmt.Fprintf(colorableErr(cmd), "%s Reopened issue #%d\n", utils.Green("✔"), issue.Number)

	return nil
}

func displayURL(urlStr string) string {
	u, err := url.Parse(urlStr)
	if err != nil {
		return urlStr
	}
	return u.Hostname() + u.Path
}
