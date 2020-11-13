package view

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmd/issue/shared"
	issueShared "github.com/cli/cli/pkg/cmd/issue/shared"
	prShared "github.com/cli/cli/pkg/cmd/pr/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/markdown"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

type ViewOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (config.Config, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)

	SelectorArg string
	WebMode     bool
	Comments    int
}

func NewCmdView(f *cmdutil.Factory, runF func(*ViewOptions) error) *cobra.Command {
	opts := &ViewOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
	}

	cmd := &cobra.Command{
		Use:   "view {<number> | <url>}",
		Short: "View an issue",
		Long: heredoc.Doc(`
			Display the title, body, and other information about an issue.

			With '--web', open the issue in a web browser instead.
    	`),
		Example: heredoc.Doc(`
    	`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if len(args) > 0 {
				opts.SelectorArg = args[0]
			}

			if runF != nil {
				return runF(opts)
			}
			return viewRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.WebMode, "web", "w", false, "Open an issue in the browser")
	cmd.Flags().IntVarP(&opts.Comments, "comments", "c", 1, "View issue comments")
	cmd.Flags().Lookup("comments").NoOptDefVal = "30"

	return cmd
}

func viewRun(opts *ViewOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	apiClient := api.NewClientFromHTTP(httpClient)

	issue, _, err := issueShared.IssueWithCommentsFromArg(apiClient, opts.BaseRepo, opts.SelectorArg, opts.Comments)
	if err != nil {
		return err
	}

	openURL := issue.URL

	if opts.WebMode {
		if opts.IO.IsStdoutTTY() {
			fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", utils.DisplayURL(openURL))
		}
		return utils.OpenInBrowser(openURL)
	}

	opts.IO.DetectTerminalTheme()

	err = opts.IO.StartPager()
	if err != nil {
		return err
	}
	defer opts.IO.StopPager()

	if opts.IO.IsStdoutTTY() {
		return printHumanIssuePreview(opts.IO, issue)
	}
	return printRawIssuePreview(opts.IO.Out, issue)
}

func printRawIssuePreview(out io.Writer, issue *api.Issue) error {
	assignees := issueAssigneeList(*issue)
	labels := shared.IssueLabelList(*issue)
	projects := issueProjectList(*issue)

	// Print empty strings for empty values so the number of metadata lines is consistent when
	// processing many issues with head and grep.
	fmt.Fprintf(out, "title:\t%s\n", issue.Title)
	fmt.Fprintf(out, "state:\t%s\n", issue.State)
	fmt.Fprintf(out, "author:\t%s\n", issue.Author.Login)
	fmt.Fprintf(out, "labels:\t%s\n", labels)
	fmt.Fprintf(out, "comments:\t%d\n", issue.Comments.TotalCount)
	fmt.Fprintf(out, "assignees:\t%s\n", assignees)
	fmt.Fprintf(out, "projects:\t%s\n", projects)
	fmt.Fprintf(out, "milestone:\t%s\n", issue.Milestone.Title)

	fmt.Fprintln(out, "--")
	fmt.Fprintln(out, issue.Body)
	fmt.Fprintln(out, "--")

	if len(issue.Comments.Nodes) > 0 {
		fmt.Fprintf(out, rawIssueComments(issue.Comments))
	}

	return nil
}

func rawIssueComments(comments api.IssueComments) string {
	var b strings.Builder
	for _, comment := range comments.Nodes {
		fmt.Fprintf(&b, rawIssueComment(comment))
	}
	return b.String()
}

func rawIssueComment(comment api.IssueComment) string {
	var b strings.Builder
	fmt.Fprintf(&b, "author:\t%s\n", comment.Author.Login)
	fmt.Fprintf(&b, "association:\t%s\n", strings.ToLower(comment.AuthorAssociation))
	fmt.Fprintln(&b, "--")
	fmt.Fprintln(&b, comment.Body)
	fmt.Fprintln(&b, "--")
	return b.String()
}

func printHumanIssuePreview(io *iostreams.IOStreams, issue *api.Issue) error {
	out := io.Out
	now := time.Now()
	ago := now.Sub(issue.CreatedAt)
	cs := io.ColorScheme()

	// Header (Title and State)
	fmt.Fprintln(out, cs.Bold(issue.Title))
	fmt.Fprintln(out, fmt.Sprintf(
		"%s • %s opened %s • %s",
		issueStateTitleWithColor(cs, issue.State),
		issue.Author.Login,
		utils.FuzzyAgo(ago),
		utils.Pluralize(issue.Comments.TotalCount, "comment"),
	))

	// Reactions
	if reactions := issue.ReactionGroups.String(); reactions != "" {
		fmt.Fprint(out, reactions)
		fmt.Fprintln(out)
	}

	// Metadata
	if assignees := issueAssigneeList(*issue); assignees != "" {
		fmt.Fprint(out, cs.Bold("Assignees: "))
		fmt.Fprintln(out, assignees)
	}
	if labels := shared.IssueLabelList(*issue); labels != "" {
		fmt.Fprint(out, cs.Bold("Labels: "))
		fmt.Fprintln(out, labels)
	}
	if projects := issueProjectList(*issue); projects != "" {
		fmt.Fprint(out, cs.Bold("Projects: "))
		fmt.Fprintln(out, projects)
	}
	if issue.Milestone.Title != "" {
		fmt.Fprint(out, cs.Bold("Milestone: "))
		fmt.Fprintln(out, issue.Milestone.Title)
	}

	// Body
	fmt.Fprintln(out)
	if issue.Body == "" {
		issue.Body = "_No description provided_"
	}
	style := markdown.GetStyle(io.TerminalTheme())
	md, err := markdown.Render(issue.Body, style, "")
	if err != nil {
		return err
	}
	fmt.Fprint(out, md)
	fmt.Fprintln(out)

	// Comments
	if issue.Comments.TotalCount > 0 {
		comments, err := issueComments(io, issue.Comments)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, comments)
	}

	// Footer
	fmt.Fprintf(out, cs.Gray("View this issue on GitHub: %s"), issue.URL)

	return nil
}

func issueComments(io *iostreams.IOStreams, comments api.IssueComments) (string, error) {
	var b strings.Builder
	cs := io.ColorScheme()
	retrievedCount := len(comments.Nodes)
	hiddenCount := comments.TotalCount - retrievedCount

	if hiddenCount > 0 {
		fmt.Fprintf(&b, cs.Gray(fmt.Sprintf("———————— Hiding %v comments ————————", hiddenCount)))
		fmt.Fprintf(&b, "\n\n\n")
	}

	for i, comment := range comments.Nodes {
		last := i+1 == retrievedCount
		cmt, err := issueComment(io, comment, last)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, cmt)
		if last {
			fmt.Fprintln(&b)
		}
	}

	if hiddenCount > 0 {
		fmt.Fprintf(&b, cs.Gray("Use --comments to view the full conversation"))
		fmt.Fprintln(&b)
	}

	return b.String(), nil
}

func issueComment(io *iostreams.IOStreams, comment api.IssueComment, newest bool) (string, error) {
	var b strings.Builder
	cs := io.ColorScheme()

	// Header
	fmt.Fprintf(&b, cs.Bold(comment.Author.Login))
	if comment.AuthorAssociation != "NONE" {
		fmt.Fprintf(&b, cs.Bold(fmt.Sprintf(" (%s)", strings.ToLower(comment.AuthorAssociation))))
	}
	fmt.Fprintf(&b, cs.Bold(fmt.Sprintf(" • %s", utils.FuzzyAgoAbbr(time.Now(), comment.CreatedAt))))
	if comment.IncludesCreatedEdit {
		fmt.Fprintf(&b, cs.Bold(" • edited"))
	}
	if newest {
		fmt.Fprintf(&b, cs.Bold(" • "))
		fmt.Fprintf(&b, cs.CyanBold("Newest comment"))
	}
	fmt.Fprintln(&b)

	// Reactions
	if reactions := comment.ReactionGroups.String(); reactions != "" {
		fmt.Fprint(&b, reactions)
		fmt.Fprintln(&b)
	}

	// Body
	if comment.Body != "" {
		style := markdown.GetStyle(io.TerminalTheme())
		md, err := markdown.Render(comment.Body, style, "")
		if err != nil {
			return "", err
		}
		fmt.Fprint(&b, md)
	}

	return b.String(), nil
}

func issueStateTitleWithColor(cs *iostreams.ColorScheme, state string) string {
	colorFunc := cs.ColorFromString(prShared.ColorForState(state))
	return colorFunc(strings.Title(strings.ToLower(state)))
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
