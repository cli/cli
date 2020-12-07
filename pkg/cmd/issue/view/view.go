package view

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/briandowns/spinner"
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
	Comments    bool
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
	cmd.Flags().BoolVarP(&opts.Comments, "comments", "c", false, "View issue comments")

	return cmd
}

func viewRun(opts *ViewOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	apiClient := api.NewClientFromHTTP(httpClient)

	issue, repo, err := issueShared.IssueFromArg(apiClient, opts.BaseRepo, opts.SelectorArg)
	if err != nil {
		return err
	}

	if opts.WebMode {
		openURL := issue.URL
		if opts.IO.IsStdoutTTY() {
			fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", utils.DisplayURL(openURL))
		}
		return utils.OpenInBrowser(openURL)
	}

	if opts.Comments {
		var s *spinner.Spinner
		if opts.IO.IsStdoutTTY() {
			s = utils.Spinner(opts.IO.ErrOut)
			utils.StartSpinner(s)
		}

		comments, err := api.CommentsForIssue(apiClient, repo, issue)
		if err != nil {
			return err
		}
		issue.Comments = *comments

		if opts.IO.IsStdoutTTY() {
			utils.StopSpinner(s)
		}
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

	if opts.Comments {
		return printRawIssueComments(opts.IO.Out, issue)
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
	return nil
}

func printRawIssueComments(out io.Writer, issue *api.Issue) error {
	var b strings.Builder
	for _, comment := range issue.Comments.Nodes {
		fmt.Fprint(&b, rawIssueComment(comment))
	}
	fmt.Fprint(out, b.String())
	return nil
}

func rawIssueComment(comment api.Comment) string {
	var b strings.Builder
	fmt.Fprintf(&b, "author:\t%s\n", comment.Author.Login)
	fmt.Fprintf(&b, "association:\t%s\n", strings.ToLower(comment.AuthorAssociation))
	fmt.Fprintf(&b, "edited:\t%t\n", comment.IncludesCreatedEdit)
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
	fmt.Fprintf(out,
		"%s • %s opened %s • %s\n",
		issueStateTitleWithColor(cs, issue.State),
		issue.Author.Login,
		utils.FuzzyAgo(ago),
		utils.Pluralize(issue.Comments.TotalCount, "comment"),
	)

	// Reactions
	if reactions := reactionGroupList(issue.ReactionGroups); reactions != "" {
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
		comments, err := issueCommentList(io, issue.Comments)
		if err != nil {
			return err
		}
		fmt.Fprint(out, comments)
	}

	// Footer
	fmt.Fprintf(out, cs.Gray("View this issue on GitHub: %s"), issue.URL)

	return nil
}

func issueCommentList(io *iostreams.IOStreams, comments api.Comments) (string, error) {
	var b strings.Builder
	cs := io.ColorScheme()
	retrievedCount := len(comments.Nodes)
	hiddenCount := comments.TotalCount - retrievedCount

	if hiddenCount > 0 {
		fmt.Fprint(&b, cs.Gray(fmt.Sprintf("———————— Not showing %s ————————", utils.Pluralize(hiddenCount, "comment"))))
		fmt.Fprintf(&b, "\n\n\n")
	}

	for i, comment := range comments.Nodes {
		last := i+1 == retrievedCount
		cmt, err := formatIssueComment(io, comment, last)
		if err != nil {
			return "", err
		}
		fmt.Fprint(&b, cmt)
		if last {
			fmt.Fprintln(&b)
		}
	}

	if hiddenCount > 0 {
		fmt.Fprint(&b, cs.Gray("Use --comments to view the full conversation"))
		fmt.Fprintln(&b)
	}

	return b.String(), nil
}

func formatIssueComment(io *iostreams.IOStreams, comment api.Comment, newest bool) (string, error) {
	var b strings.Builder
	cs := io.ColorScheme()

	// Header
	fmt.Fprint(&b, cs.Bold(comment.Author.Login))
	if comment.AuthorAssociation != "NONE" {
		fmt.Fprint(&b, cs.Bold(fmt.Sprintf(" (%s)", strings.ToLower(comment.AuthorAssociation))))
	}
	fmt.Fprint(&b, cs.Bold(fmt.Sprintf(" • %s", utils.FuzzyAgoAbbr(time.Now(), comment.CreatedAt))))
	if comment.IncludesCreatedEdit {
		fmt.Fprint(&b, cs.Bold(" • edited"))
	}
	if newest {
		fmt.Fprint(&b, cs.Bold(" • "))
		fmt.Fprint(&b, cs.CyanBold("Newest comment"))
	}
	fmt.Fprintln(&b)

	// Reactions
	if reactions := reactionGroupList(comment.ReactionGroups); reactions != "" {
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

func reactionGroupList(rgs api.ReactionGroups) string {
	var rs []string

	for _, rg := range rgs {
		if r := formatReactionGroup(rg); r != "" {
			rs = append(rs, r)
		}
	}

	return strings.Join(rs, " • ")
}

func formatReactionGroup(rg api.ReactionGroup) string {
	c := rg.Count()
	if c == 0 {
		return ""
	}
	e := rg.Emoji()
	if e == "" {
		return ""
	}
	return fmt.Sprintf("%v %s", c, e)
}
