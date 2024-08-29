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
	presentationIssue *PresentationIssue
	colorScheme       *iostreams.ColorScheme
	IO                *iostreams.IOStreams
	time              time.Time
	baseRepo          ghrepo.Interface
}

type PresentationIssue struct {
	Title          string
	Number         int
	CreatedAt      time.Time
	Comments       api.Comments
	Author         string
	State          string
	StateReason    string
	Reactions      string
	AssigneesList  string
	LabelsList     string
	ProjectsList   string
	MilestoneTitle string
	Body           string
	URL            string
}

func NewIssuePrintFormatter(presentationIssue *PresentationIssue, IO *iostreams.IOStreams, timeNow time.Time, baseRepo ghrepo.Interface) *IssuePrintFormatter {
	return &IssuePrintFormatter{
		presentationIssue: presentationIssue,
		colorScheme:       IO.ColorScheme(),
		IO:                IO,
		time:              timeNow,
		baseRepo:          baseRepo,
	}
}

func apiIssueToPresentationIssue(issue *api.Issue, colorScheme *iostreams.ColorScheme) (*PresentationIssue, error) {
	presentationIssue := &PresentationIssue{
		Title:         issue.Title,
		Number:        issue.Number,
		CreatedAt:     issue.CreatedAt,
		Comments:      issue.Comments,
		Author:        issue.Author.Login,
		State:         issue.State,
		StateReason:   issue.StateReason,
		Reactions:     prShared.ReactionGroupList(issue.ReactionGroups),
		AssigneesList: getAssigneeListString(issue.Assignees),
		// It feels weird to add color here...
		LabelsList:   getColorizedLabelsList(issue.Labels, colorScheme),
		ProjectsList: getProjectListString(issue.ProjectCards, issue.ProjectItems),
		Body:         issue.Body,
		URL:          issue.URL,
	}

	if issue.Milestone != nil {
		presentationIssue.MilestoneTitle = issue.Milestone.Title
	}

	return presentationIssue, nil
}

func getProjectListString(projectCards api.ProjectCards, projectItems api.ProjectItems) string {
	if len(projectCards.Nodes) == 0 {
		return ""
	}

	projectNames := make([]string, 0, len(projectCards.Nodes))
	for _, project := range projectCards.Nodes {
		colName := project.Column.Name
		if colName == "" {
			colName = "Awaiting triage"
		}
		projectNames = append(projectNames, fmt.Sprintf("%s (%s)", project.Project.Name, colName))
	}

	list := strings.Join(projectNames, ", ")
	if projectCards.TotalCount > len(projectCards.Nodes) {
		list += ", …"
	}
	return list
}

func getAssigneeListString(issueAssignees api.Assignees) string {
	if len(issueAssignees.Nodes) == 0 {
		return ""
	}

	AssigneeNames := make([]string, 0, len(issueAssignees.Nodes))
	for _, assignee := range issueAssignees.Nodes {
		AssigneeNames = append(AssigneeNames, assignee.Login)
	}

	list := strings.Join(AssigneeNames, ", ")
	if issueAssignees.TotalCount > len(issueAssignees.Nodes) {
		list += ", …"
	}
	return list
}

func getColorizedLabelsList(issueLabels api.Labels, colorScheme *iostreams.ColorScheme) string {
	labelNames := make([]string, len(issueLabels.Nodes))
	for j, label := range issueLabels.Nodes {
		if colorScheme == nil {
			labelNames[j] = label.Name
		} else {
			labelNames[j] = colorScheme.HexToRGB(label.Color, label.Name)
		}
	}

	return strings.Join(labelNames, ", ")
}

func (ipf *IssuePrintFormatter) renderHumanIssuePreview(isCommentsPreview bool) error {

	// I think I'd like to make this easier to understand what the output should look like.
	// That's probably doable by removing these helpers and just using a formatted string.
	// I might experiment with that later.

	// header (Title and State)
	ipf.header()
	// Reactions
	ipf.reactions()
	// Metadata
	ipf.assigneeList()
	ipf.labelList()
	ipf.projectList()

	ipf.milestone()

	// Body
	err := ipf.body()
	if err != nil {
		return err
	}

	// Comments
	err = ipf.comments(isCommentsPreview)
	if err != nil {
		return err
	}

	// Footer
	ipf.footer()

	return nil
}

func (i *IssuePrintFormatter) header() {
	fmt.Fprintf(i.IO.Out, "%s %s#%d\n", i.colorScheme.Bold(i.presentationIssue.Title), ghrepo.FullName(i.baseRepo), i.presentationIssue.Number)
	fmt.Fprintf(i.IO.Out,
		"%s • %s opened %s • %s\n",
		i.issueStateTitleWithColor(),
		i.presentationIssue.Author,
		text.FuzzyAgo(i.time, i.presentationIssue.CreatedAt),
		text.Pluralize(i.presentationIssue.Comments.TotalCount, "comment"),
	)
}

func (i *IssuePrintFormatter) issueStateTitleWithColor() string {
	colorFunc := i.colorScheme.ColorFromString(prShared.ColorForIssueState(i.presentationIssue.State, i.presentationIssue.StateReason))
	state := "Open"
	if i.presentationIssue.State == "CLOSED" {
		state = "Closed"
	}
	return colorFunc(state)
}

func (i *IssuePrintFormatter) reactions() {
	if i.presentationIssue.Reactions != "" {
		fmt.Fprint(i.IO.Out, i.presentationIssue.Reactions)
		fmt.Fprintln(i.IO.Out)
	}
}

func (i *IssuePrintFormatter) assigneeList() {
	assignees := i.presentationIssue.AssigneesList
	if assignees != "" {
		fmt.Fprint(i.IO.Out, i.colorScheme.Bold("Assignees: "))
		fmt.Fprintln(i.IO.Out, assignees)
	}
}

func (i *IssuePrintFormatter) labelList() {
	labels := i.presentationIssue.LabelsList
	if labels != "" {
		fmt.Fprint(i.IO.Out, i.colorScheme.Bold("Labels: "))
		fmt.Fprintln(i.IO.Out, labels)
	}
}

func (i *IssuePrintFormatter) projectList() {
	projects := i.presentationIssue.ProjectsList
	if projects != "" {
		fmt.Fprint(i.IO.Out, i.colorScheme.Bold("Projects: "))
		fmt.Fprintln(i.IO.Out, projects)
	}
}

func (i *IssuePrintFormatter) milestone() {
	if i.presentationIssue.MilestoneTitle != "" {
		fmt.Fprint(i.IO.Out, i.colorScheme.Bold("Milestone: "))
		fmt.Fprintln(i.IO.Out, i.presentationIssue.MilestoneTitle)
	}
}

func (i *IssuePrintFormatter) body() error {
	var md string
	var err error
	body := i.presentationIssue.Body
	if body == "" {
		md = fmt.Sprintf("\n  %s\n\n", i.colorScheme.Gray("No description provided"))
	} else {
		md, err = markdown.Render(body,
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
	if i.presentationIssue.Comments.TotalCount > 0 {
		comments, err := prShared.CommentList(i.IO, i.presentationIssue.Comments, api.PullRequestReviews{}, isPreview)
		if err != nil {
			return err
		}
		fmt.Fprint(i.IO.Out, comments)
	}
	return nil
}

func (i *IssuePrintFormatter) footer() {
	fmt.Fprintf(i.IO.Out, i.colorScheme.Gray("View this issue on GitHub: %s\n"), i.presentationIssue.URL)
}
