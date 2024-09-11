package view

import (
	"fmt"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/text"
	prShared "github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/markdown"
)

type RichIssuePrinter struct {
	IO       *iostreams.IOStreams
	TimeNow  time.Time
	Comments bool
}

// Print outputs an issue to the terminal for TTY use cases.
func (p *RichIssuePrinter) Print(pi PresentationIssue, repo ghrepo.Interface) error {

	// header (Title and State)
	p.header(pi, repo)
	// Reactions
	p.reactions(pi)
	// Metadata
	p.assigneeList(pi)
	p.labelList(pi)
	p.projectList(pi)

	p.milestone(pi)

	// Body
	err := p.body(pi)
	if err != nil {
		return err
	}

	// Comments
	isCommentsPreview := !p.Comments
	err = p.comments(pi, isCommentsPreview)
	if err != nil {
		return err
	}

	// Footer
	p.footer(pi)

	return nil
}

func (p *RichIssuePrinter) header(pi PresentationIssue, repo ghrepo.Interface) {
	fmt.Fprintf(p.IO.Out, "%s %s#%d\n", p.IO.ColorScheme().Bold(pi.Title), ghrepo.FullName(repo), pi.Number)
	fmt.Fprintf(p.IO.Out,
		"%s • %s opened %s • %s\n",
		p.issueStateTitleWithColor(pi.State, pi.StateReason),
		pi.Author,
		text.FuzzyAgo(p.TimeNow, pi.CreatedAt),
		text.Pluralize(pi.Comments.TotalCount, "comment"),
	)
}

func (p *RichIssuePrinter) issueStateTitleWithColor(state string, stateReason string) string {
	colorFunc := p.IO.ColorScheme().ColorFromString(prShared.ColorForIssueState(state, stateReason))
	formattedState := "Open"
	if state == "CLOSED" {
		formattedState = "Closed"
	}
	return colorFunc(formattedState)
}

func (p *RichIssuePrinter) reactions(pi PresentationIssue) {
	if pi.Reactions != "" {
		fmt.Fprint(p.IO.Out, pi.Reactions)
		fmt.Fprintln(p.IO.Out)
	}
}

func (p *RichIssuePrinter) assigneeList(pi PresentationIssue) {
	assignees := pi.AssigneesList
	if assignees != "" {
		fmt.Fprint(p.IO.Out, p.IO.ColorScheme().Bold("Assignees: "))
		fmt.Fprintln(p.IO.Out, assignees)
	}
}

func (p *RichIssuePrinter) labelList(pi PresentationIssue) {
	labels := pi.LabelsList
	if labels != "" {
		fmt.Fprint(p.IO.Out, p.IO.ColorScheme().Bold("Labels: "))
		fmt.Fprintln(p.IO.Out, labels)
	}
}

func (p *RichIssuePrinter) projectList(pi PresentationIssue) {
	projects := pi.ProjectsList
	if projects != "" {
		fmt.Fprint(p.IO.Out, p.IO.ColorScheme().Bold("Projects: "))
		fmt.Fprintln(p.IO.Out, projects)
	}
}

func (p *RichIssuePrinter) milestone(pi PresentationIssue) {
	if pi.MilestoneTitle != "" {
		fmt.Fprint(p.IO.Out, p.IO.ColorScheme().Bold("Milestone: "))
		fmt.Fprintln(p.IO.Out, pi.MilestoneTitle)
	}
}

func (p *RichIssuePrinter) body(pi PresentationIssue) error {
	var md string
	var err error
	body := pi.Body
	if body == "" {
		md = fmt.Sprintf("\n  %s\n\n", p.IO.ColorScheme().Gray("No description provided"))
	} else {
		md, err = markdown.Render(body,
			markdown.WithTheme(p.IO.TerminalTheme()),
			markdown.WithWrap(p.IO.TerminalWidth()))
		if err != nil {
			return err
		}
	}
	fmt.Fprintf(p.IO.Out, "\n%s\n", md)
	return nil
}

func (p *RichIssuePrinter) comments(pi PresentationIssue, isPreview bool) error {
	if pi.Comments.TotalCount > 0 {
		comments, err := prShared.CommentList(p.IO, pi.Comments, api.PullRequestReviews{}, isPreview)
		if err != nil {
			return err
		}
		fmt.Fprint(p.IO.Out, comments)
	}
	return nil
}

func (p *RichIssuePrinter) footer(pi PresentationIssue) {
	fmt.Fprintf(p.IO.Out, p.IO.ColorScheme().Gray("View this issue on GitHub: %s\n"), pi.URL)
}
