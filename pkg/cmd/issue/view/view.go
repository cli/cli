package view

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/text"
	issueShared "github.com/cli/cli/v2/pkg/cmd/issue/shared"
	prShared "github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/markdown"
	"github.com/cli/cli/v2/pkg/set"
	"github.com/spf13/cobra"
)

type ViewOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Browser    browser.Browser

	SelectorArg string
	WebMode     bool
	Comments    bool
	Exporter    cmdutil.Exporter

	Now func() time.Time
}

func NewCmdView(f *cmdutil.Factory, runF func(*ViewOptions) error) *cobra.Command {
	opts := &ViewOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Browser:    f.Browser,
		Now:        time.Now,
	}

	cmd := &cobra.Command{
		Use:   "view {<number> | <url>}",
		Short: "View an issue",
		Long: heredoc.Doc(`
			Display the title, body, and other information about an issue.

			With '--web', open the issue in a web browser instead.
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
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, api.IssueFields)

	return cmd
}

var defaultFields = []string{
	"number", "url", "state", "createdAt", "title", "body", "author", "milestone",
	"assignees", "labels", "projectCards", "reactionGroups", "lastComment", "stateReason",
}

func viewRun(opts *ViewOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	lookupFields := set.NewStringSet()
	if opts.Exporter != nil {
		lookupFields.AddValues(opts.Exporter.Fields())
	} else if opts.WebMode {
		lookupFields.Add("url")
	} else {
		lookupFields.AddValues(defaultFields)
		if opts.Comments {
			lookupFields.Add("comments")
			lookupFields.Remove("lastComment")
		}
	}

	opts.IO.DetectTerminalTheme()

	opts.IO.StartProgressIndicator()
	issue, err := findIssue(httpClient, opts.BaseRepo, opts.SelectorArg, lookupFields.ToSlice())
	opts.IO.StopProgressIndicator()
	if err != nil {
		var loadErr *issueShared.PartialLoadError
		if opts.Exporter == nil && errors.As(err, &loadErr) {
			fmt.Fprintf(opts.IO.ErrOut, "warning: %s\n", loadErr.Error())
		} else {
			return err
		}
	}

	if opts.WebMode {
		openURL := issue.URL
		if opts.IO.IsStdoutTTY() {
			fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", text.DisplayURL(openURL))
		}
		return opts.Browser.Browse(openURL)
	}

	if err := opts.IO.StartPager(); err != nil {
		fmt.Fprintf(opts.IO.ErrOut, "error starting pager: %v\n", err)
	}
	defer opts.IO.StopPager()

	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, issue)
	}

	if opts.IO.IsStdoutTTY() {
		return printHumanIssuePreview(opts, issue)
	}

	if opts.Comments {
		fmt.Fprint(opts.IO.Out, prShared.RawCommentList(issue.Comments, api.PullRequestReviews{}))
		return nil
	}

	return printRawIssuePreview(opts.IO.Out, issue)
}

func findIssue(client *http.Client, baseRepoFn func() (ghrepo.Interface, error), selector string, fields []string) (*api.Issue, error) {
	fieldSet := set.NewStringSet()
	fieldSet.AddValues(fields)
	fieldSet.Add("id")

	issue, repo, err := issueShared.IssueFromArgWithFields(client, baseRepoFn, selector, fieldSet.ToSlice())
	if err != nil {
		return issue, err
	}

	if fieldSet.Contains("comments") {
		// FIXME: this re-fetches the comments connection even though the initial set of 100 were
		// fetched in the previous request.
		err = preloadIssueComments(client, repo, issue)
	}
	return issue, err
}

func printRawIssuePreview(out io.Writer, issue *api.Issue) error {
	assignees := issueAssigneeList(*issue)
	labels := issueLabelList(issue, nil)
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
	var milestoneTitle string
	if issue.Milestone != nil {
		milestoneTitle = issue.Milestone.Title
	}
	fmt.Fprintf(out, "milestone:\t%s\n", milestoneTitle)
	fmt.Fprintf(out, "number:\t%d\n", issue.Number)
	fmt.Fprintln(out, "--")
	fmt.Fprintln(out, issue.Body)
	return nil
}

func printHumanIssuePreview(opts *ViewOptions, issue *api.Issue) error {
	out := opts.IO.Out
	cs := opts.IO.ColorScheme()

	// Header (Title and State)
	fmt.Fprintf(out, "%s #%d\n", cs.Bold(issue.Title), issue.Number)
	fmt.Fprintf(out,
		"%s • %s opened %s • %s\n",
		issueStateTitleWithColor(cs, issue),
		issue.Author.Login,
		text.FuzzyAgo(opts.Now(), issue.CreatedAt),
		text.Pluralize(issue.Comments.TotalCount, "comment"),
	)

	// Reactions
	if reactions := prShared.ReactionGroupList(issue.ReactionGroups); reactions != "" {
		fmt.Fprint(out, reactions)
		fmt.Fprintln(out)
	}

	// Metadata
	if assignees := issueAssigneeList(*issue); assignees != "" {
		fmt.Fprint(out, cs.Bold("Assignees: "))
		fmt.Fprintln(out, assignees)
	}
	if labels := issueLabelList(issue, cs); labels != "" {
		fmt.Fprint(out, cs.Bold("Labels: "))
		fmt.Fprintln(out, labels)
	}
	if projects := issueProjectList(*issue); projects != "" {
		fmt.Fprint(out, cs.Bold("Projects: "))
		fmt.Fprintln(out, projects)
	}
	if issue.Milestone != nil {
		fmt.Fprint(out, cs.Bold("Milestone: "))
		fmt.Fprintln(out, issue.Milestone.Title)
	}

	// Body
	var md string
	var err error
	if issue.Body == "" {
		md = fmt.Sprintf("\n  %s\n\n", cs.Gray("No description provided"))
	} else {
		md, err = markdown.Render(issue.Body,
			markdown.WithTheme(opts.IO.TerminalTheme()),
			markdown.WithWrap(opts.IO.TerminalWidth()))
		if err != nil {
			return err
		}
	}
	fmt.Fprintf(out, "\n%s\n", md)

	// Comments
	if issue.Comments.TotalCount > 0 {
		preview := !opts.Comments
		comments, err := prShared.CommentList(opts.IO, issue.Comments, api.PullRequestReviews{}, preview)
		if err != nil {
			return err
		}
		fmt.Fprint(out, comments)
	}

	// Footer
	fmt.Fprintf(out, cs.Gray("View this issue on GitHub: %s\n"), issue.URL)

	return nil
}

func issueStateTitleWithColor(cs *iostreams.ColorScheme, issue *api.Issue) string {
	colorFunc := cs.ColorFromString(prShared.ColorForIssueState(*issue))
	state := "Open"
	if issue.State == "CLOSED" {
		state = "Closed"
	}
	return colorFunc(state)
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

func issueLabelList(issue *api.Issue, cs *iostreams.ColorScheme) string {
	if len(issue.Labels.Nodes) == 0 {
		return ""
	}

	labelNames := make([]string, len(issue.Labels.Nodes))
	for i, label := range issue.Labels.Nodes {
		if cs == nil {
			labelNames[i] = label.Name
		} else {
			labelNames[i] = cs.HexToRGB(label.Color, label.Name)
		}
	}

	return strings.Join(labelNames, ", ")
}
