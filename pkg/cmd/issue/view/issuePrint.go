package view

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/text"
	prShared "github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/markdown"
)

type IssuePrint struct {
	issue       *api.Issue
	colorScheme *iostreams.ColorScheme
	IO          *iostreams.IOStreams
	time        time.Time
	baseRepo    ghrepo.Interface
}

func (i *IssuePrint) rawPreview() error {
	assignees := i.getAssigneeList()
	labels := i.getLabelList()
	projects := i.getProjectList()

	// Print empty strings for empty values so the number of metadata lines is consistent when
	// processing many issues with head and grep.
	fmt.Fprintf(i.IO.Out, "title:\t%s\n", i.issue.Title)
	fmt.Fprintf(i.IO.Out, "state:\t%s\n", i.issue.State)
	fmt.Fprintf(i.IO.Out, "author:\t%s\n", i.issue.Author.Login)
	fmt.Fprintf(i.IO.Out, "labels:\t%s\n", labels)
	fmt.Fprintf(i.IO.Out, "comments:\t%d\n", i.issue.Comments.TotalCount)
	fmt.Fprintf(i.IO.Out, "assignees:\t%s\n", assignees)
	fmt.Fprintf(i.IO.Out, "projects:\t%s\n", projects)
	var milestoneTitle string
	if i.issue.Milestone != nil {
		milestoneTitle = i.issue.Milestone.Title
	}
	fmt.Fprintf(i.IO.Out, "milestone:\t%s\n", milestoneTitle)
	fmt.Fprintf(i.IO.Out, "number:\t%d\n", i.issue.Number)
	fmt.Fprintln(i.IO.Out, "--")
	fmt.Fprintln(i.IO.Out, i.issue.Body)
	return nil
}

func (i *IssuePrint) humanPreview(isCommentsPreview bool) error {

	// header (Title and State)
	i.header()
	// Reactions
	i.reactions()
	// Metadata
	i.assigneeList()
	i.labelList()
	i.projectList()
	i.milestone()

	// Body
	err := i.body()
	if err != nil {
		return err
	}

	// Comments
	err = i.comments(isCommentsPreview)
	if err != nil {
		return err
	}

	// Footer
	i.footer()

	return nil
}

func (i *IssuePrint) header() {
	fmt.Fprintf(i.IO.Out, "%s %s#%d\n", i.colorScheme.Bold(i.issue.Title), ghrepo.FullName(i.baseRepo), i.issue.Number)
	fmt.Fprintf(i.IO.Out,
		"%s • %s opened %s • %s\n",
		i.issueStateTitleWithColor(),
		i.issue.Author.Login,
		text.FuzzyAgo(i.time, i.issue.CreatedAt),
		text.Pluralize(i.issue.Comments.TotalCount, "comment"),
	)
}

func (i *IssuePrint) issueStateTitleWithColor() string {
	colorFunc := i.colorScheme.ColorFromString(prShared.ColorForIssueState(*i.issue))
	state := "Open"
	if i.issue.State == "CLOSED" {
		state = "Closed"
	}
	return colorFunc(state)
}

func (i *IssuePrint) reactions() {
	if reactions := prShared.ReactionGroupList(i.issue.ReactionGroups); reactions != "" {
		fmt.Fprint(i.IO.Out, reactions)
		fmt.Fprintln(i.IO.Out)
	}
}

func (i *IssuePrint) assigneeList() {
	if assignees := i.getAssigneeList(); assignees != "" {
		fmt.Fprint(i.IO.Out, i.colorScheme.Bold("Assignees: "))
		fmt.Fprintln(i.IO.Out, assignees)
	}
}

func (i *IssuePrint) getAssigneeList() string {
	if len(i.issue.Assignees.Nodes) == 0 {
		return ""
	}

	AssigneeNames := make([]string, 0, len(i.issue.Assignees.Nodes))
	for _, assignee := range i.issue.Assignees.Nodes {
		AssigneeNames = append(AssigneeNames, assignee.Login)
	}

	list := strings.Join(AssigneeNames, ", ")
	if i.issue.Assignees.TotalCount > len(i.issue.Assignees.Nodes) {
		list += ", …"
	}
	return list
}

func (i *IssuePrint) labelList() {
	if labels := i.getLabelList(); labels != "" {
		fmt.Fprint(i.IO.Out, i.colorScheme.Bold("Labels: "))
		fmt.Fprintln(i.IO.Out, labels)
	}
}

func (i *IssuePrint) projectList() {
	if projects := i.getProjectList(); projects != "" {
		fmt.Fprint(i.IO.Out, i.colorScheme.Bold("Projects: "))
		fmt.Fprintln(i.IO.Out, projects)
	}
}

func (i *IssuePrint) getProjectList() string {
	if len(i.issue.ProjectCards.Nodes) == 0 {
		return ""
	}

	projectNames := make([]string, 0, len(i.issue.ProjectCards.Nodes))
	for _, project := range i.issue.ProjectCards.Nodes {
		colName := project.Column.Name
		if colName == "" {
			colName = "Awaiting triage"
		}
		projectNames = append(projectNames, fmt.Sprintf("%s (%s)", project.Project.Name, colName))
	}

	list := strings.Join(projectNames, ", ")
	if i.issue.ProjectCards.TotalCount > len(i.issue.ProjectCards.Nodes) {
		list += ", …"
	}
	return list
}

func (i *IssuePrint) getLabelList() string {
	if len(i.issue.Labels.Nodes) == 0 {
		return ""
	}

	// ignore case sort
	sort.SliceStable(i.issue.Labels.Nodes, func(j, k int) bool {
		return strings.ToLower(i.issue.Labels.Nodes[j].Name) < strings.ToLower(i.issue.Labels.Nodes[k].Name)
	})

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

func (i *IssuePrint) milestone() {
	if i.issue.Milestone != nil {
		fmt.Fprint(i.IO.Out, i.colorScheme.Bold("Milestone: "))
		fmt.Fprintln(i.IO.Out, i.issue.Milestone.Title)
	}
}

func (i *IssuePrint) body() error {
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

func (i *IssuePrint) comments(isPreview bool) error {
	if i.issue.Comments.TotalCount > 0 {
		comments, err := prShared.CommentList(i.IO, i.issue.Comments, api.PullRequestReviews{}, isPreview)
		if err != nil {
			return err
		}
		fmt.Fprint(i.IO.Out, comments)
	}
	return nil
}

func (i *IssuePrint) footer() {
	fmt.Fprintf(i.IO.Out, i.colorScheme.Gray("View this issue on GitHub: %s\n"), i.issue.URL)
}
