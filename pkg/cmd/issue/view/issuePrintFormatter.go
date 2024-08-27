package view

import (
	"fmt"
	"strings"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/text"
	prShared "github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/markdown"
)

type IssuePrintFormatter struct {
	issue       *api.Issue
	colorScheme *iostreams.ColorScheme
	IO          *iostreams.IOStreams
	time        time.Time
	baseRepo    ghrepo.Interface
}

func NewIssuePrintFormatter(issue *api.Issue, IO *iostreams.IOStreams, timeNow time.Time, baseRepo ghrepo.Interface) *IssuePrintFormatter {
	return &IssuePrintFormatter{
		issue:       issue,
		colorScheme: IO.ColorScheme(),
		IO:          IO,
		time:        timeNow,
		baseRepo:    baseRepo,
	}
}

func (i *IssuePrintFormatter) header() {
	fmt.Fprintf(i.IO.Out, "%s %s#%d\n", i.colorScheme.Bold(i.issue.Title), ghrepo.FullName(i.baseRepo), i.issue.Number)
	fmt.Fprintf(i.IO.Out,
		"%s • %s opened %s • %s\n",
		i.issueStateTitleWithColor(),
		i.issue.Author.Login,
		text.FuzzyAgo(i.time, i.issue.CreatedAt),
		text.Pluralize(i.issue.Comments.TotalCount, "comment"),
	)
}

func (i *IssuePrintFormatter) issueStateTitleWithColor() string {
	colorFunc := i.colorScheme.ColorFromString(prShared.ColorForIssueState(*i.issue))
	state := "Open"
	if i.issue.State == "CLOSED" {
		state = "Closed"
	}
	return colorFunc(state)
}

func (i *IssuePrintFormatter) reactions() {
	if reactions := prShared.ReactionGroupList(i.issue.ReactionGroups); reactions != "" {
		fmt.Fprint(i.IO.Out, reactions)
		fmt.Fprintln(i.IO.Out)
	}
}

func (i *IssuePrintFormatter) assigneeList() {
	if assignees := i.issue.GetAssigneeListString(); assignees != "" {
		fmt.Fprint(i.IO.Out, i.colorScheme.Bold("Assignees: "))
		fmt.Fprintln(i.IO.Out, assignees)
	}
}

func (i *IssuePrintFormatter) labelList() {
	if labels := i.getColorizedLabelsList(); labels != "" {
		fmt.Fprint(i.IO.Out, i.colorScheme.Bold("Labels: "))
		fmt.Fprintln(i.IO.Out, labels)
	}
}

func (i *IssuePrintFormatter) projectList() {
	if projects := i.issue.GetProjectListString(); projects != "" {
		fmt.Fprint(i.IO.Out, i.colorScheme.Bold("Projects: "))
		fmt.Fprintln(i.IO.Out, projects)
	}
}

func (i *IssuePrintFormatter) getColorizedLabelsList() string {
	labelNames := make([]string, len(i.issue.Labels.Nodes))
	for j, label := range i.issue.Labels.Nodes {
		if i.colorScheme == nil {
			labelNames[j] = label.Name
		} else {
			labelNames[j] = i.colorScheme.HexToRGB(label.Color, label.Name)
		}
	}

	return strings.Join(labelNames, ", ")
}

func (i *IssuePrintFormatter) milestone() {
	if i.issue.Milestone != nil {
		fmt.Fprint(i.IO.Out, i.colorScheme.Bold("Milestone: "))
		fmt.Fprintln(i.IO.Out, i.issue.Milestone.Title)
	}
}

func (i *IssuePrintFormatter) body() error {
	var md string
	var err error
	if i.issue.Body == "" {
		md = fmt.Sprintf("\n  %s\n\n", i.colorScheme.Gray("No description provided"))
	} else {
		md, err = markdown.Render(i.issue.Body,
			markdown.WithTheme(i.IO.TerminalTheme()),
			markdown.WithWrap(i.IO.TerminalWidth()))
		if err != nil {
			return err
		}
	}
	fmt.Fprintf(i.IO.Out, "\n%s\n", md)
	return nil
}

func (i *IssuePrintFormatter) comments(isPreview bool) error {
	if i.issue.Comments.TotalCount > 0 {
		comments, err := prShared.CommentList(i.IO, i.issue.Comments, api.PullRequestReviews{}, isPreview)
		if err != nil {
			return err
		}
		fmt.Fprint(i.IO.Out, comments)
	}
	return nil
}

func (i *IssuePrintFormatter) footer() {
	fmt.Fprintf(i.IO.Out, i.colorScheme.Gray("View this issue on GitHub: %s\n"), i.issue.URL)
}
