package view

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/cli/cli/v2/api"
	prShared "github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/iostreams"
)

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

func MapApiIssueToPresentationIssue(issue *api.Issue, colorScheme *iostreams.ColorScheme) (PresentationIssue, error) {
	presentationIssue := PresentationIssue{
		Title:         issue.Title,
		Number:        issue.Number,
		CreatedAt:     issue.CreatedAt,
		Comments:      issue.Comments,
		Author:        issue.Author.Login,
		State:         issue.State,
		StateReason:   issue.StateReason,
		Reactions:     prShared.ReactionGroupList(issue.ReactionGroups),
		AssigneesList: getAssigneeListString(issue.Assignees),
		LabelsList:    getColorizedLabelsList(sortAlphabeticallyIgnoreCase(issue.Labels), colorScheme),
		ProjectsList:  getProjectListString(issue.ProjectCards, issue.ProjectItems),
		Body:          issue.Body,
		URL:           issue.URL,
	}

	if issue.Milestone != nil {
		presentationIssue.MilestoneTitle = issue.Milestone.Title
	}

	return presentationIssue, nil
}

func getProjectListString(projectCards api.ProjectCards, projectItems api.ProjectItems) string {
	if len(projectCards.Nodes) == 0 && len(projectItems.Nodes) == 0 {
		return ""
	}

	projectNames := make([]string, len(projectCards.Nodes)+len(projectItems.Nodes))
	for i, project := range projectCards.Nodes {
		colName := project.Column.Name
		if colName == "" {
			colName = "Awaiting triage"
		}
		projectNames[i] = fmt.Sprintf("%s (%s)", project.Project.Name, colName)
	}

	for i, project := range projectItems.Nodes {
		statusName := project.Status.Name
		if statusName == "" {
			statusName = "Backlog"
		}
		projectNames[i+len(projectCards.Nodes)] = fmt.Sprintf("%s (%s)", project.Project.Title, statusName)
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

func sortAlphabeticallyIgnoreCase(l api.Labels) api.Labels {
	slices.SortStableFunc(l.Nodes, func(a, b api.IssueLabel) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return l
}
