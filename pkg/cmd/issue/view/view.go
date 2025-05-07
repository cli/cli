package view

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/browser"
	fd "github.com/cli/cli/v2/internal/featuredetection"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmd/issue/shared"
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
	Detector   fd.Detector

	IssueNumber int
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
		Long: heredoc.Docf(`
			Display the title, body, and other information about an issue.

			With %[1]s--web%[1]s flag, open the issue in a web browser instead.
		`, "`"),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			issueNumber, baseRepo, err := shared.ParseIssueFromArg(args[0])
			if err != nil {
				return err
			}

			// If the args provided the base repo then use that directly.
			if baseRepo, present := baseRepo.Value(); present {
				opts.BaseRepo = func() (ghrepo.Interface, error) {
					return baseRepo, nil
				}
			} else {
				// support `-R, --repo` override
				opts.BaseRepo = f.BaseRepo
			}

			opts.IssueNumber = issueNumber

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
	"assignees", "labels", "reactionGroups", "lastComment", "stateReason",
}

func viewRun(opts *ViewOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	baseRepo, err := opts.BaseRepo()
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

		// TODO projectsV1Deprecation
		// Remove this section as we should no longer add projectCards
		if opts.Detector == nil {
			cachedClient := api.NewCachedHTTPClient(httpClient, time.Hour*24)
			opts.Detector = fd.NewDetector(cachedClient, baseRepo.RepoHost())
		}

		projectsV1Support := opts.Detector.ProjectsV1()
		if projectsV1Support == gh.ProjectsV1Supported {
			lookupFields.Add("projectCards")
		}
	}

	opts.IO.DetectTerminalTheme()

	opts.IO.StartProgressIndicator()
	defer opts.IO.StopProgressIndicator()

	lookupFields.Add("id")

	issue, err := issueShared.FindIssueOrPR(httpClient, baseRepo, opts.IssueNumber, lookupFields.ToSlice())
	if err != nil {
		return err
	}

	if lookupFields.Contains("comments") {
		// FIXME: this re-fetches the comments connection even though the initial set of 100 were
		// fetched in the previous request.
		err := preloadIssueComments(httpClient, baseRepo, issue)
		if err != nil {
			return err
		}
	}

	if lookupFields.Contains("closedByPullRequestsReferences") {
		err := preloadClosedByPullRequestsReferences(httpClient, baseRepo, issue)
		if err != nil {
			return err
		}
	}

	opts.IO.StopProgressIndicator()

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
		return printHumanIssuePreview(opts, baseRepo, issue)
	}

	if opts.Comments {
		fmt.Fprint(opts.IO.Out, prShared.RawCommentList(issue.Comments, api.PullRequestReviews{}))
		return nil
	}

	return printRawIssuePreview(opts.IO.Out, issue)
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

func printHumanIssuePreview(opts *ViewOptions, baseRepo ghrepo.Interface, issue *api.Issue) error {
	out := opts.IO.Out
	cs := opts.IO.ColorScheme()

	// Header (Title and State)
	fmt.Fprintf(out, "%s %s#%d\n", cs.Bold(issue.Title), ghrepo.FullName(baseRepo), issue.Number)
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
		md = fmt.Sprintf("\n  %s\n\n", cs.Muted("No description provided"))
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
	fmt.Fprintf(out, cs.Muted("View this issue on GitHub: %s\n"), issue.URL)

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

	// ignore case sort
	sort.SliceStable(issue.Labels.Nodes, func(i, j int) bool {
		return strings.ToLower(issue.Labels.Nodes[i].Name) < strings.ToLower(issue.Labels.Nodes[j].Name)
	})

	labelNames := make([]string, len(issue.Labels.Nodes))
	for i, label := range issue.Labels.Nodes {
		if cs == nil {
			labelNames[i] = label.Name
		} else {
			labelNames[i] = cs.Label(label.Color, label.Name)
		}
	}

	return strings.Join(labelNames, ", ")
}
