package view

import (
	"testing"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
)

func Test_MapApiIssueToPresentationIssue(t *testing.T) {
	createdAt, err := time.Parse(time.RFC3339, "2022-01-01T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}

	tests := map[string]struct {
		issue  *api.Issue
		expect PresentationIssue
	}{
		"basic integration test": {
			issue: &api.Issue{
				Title:  "Title",
				Number: 123,
				Comments: api.Comments{
					Nodes: []api.Comment{
						{
							Author: api.CommentAuthor{
								Login: "monalisa",
							},
							Body:           "comment body 1",
							ReactionGroups: api.ReactionGroups{},
						},
					},
					TotalCount: 1,
				},
				State:       "OPEN",
				StateReason: "",
				URL:         "github.com/OWNER/REPO/issues/123",
				Author: api.Author{
					Login: "author",
					Name:  "Octo Cat",
					ID:    "321",
				},
				Assignees: api.Assignees{
					Nodes: []api.GitHubUser{
						{
							Login: "assignee",
							Name:  "Octo Cat",
							ID:    "321",
						},
					},
					TotalCount: 1,
				},
				Labels: api.Labels{
					Nodes: []api.IssueLabel{
						{
							Name:  "bug",
							Color: "fc0303",
						},
					},
					TotalCount: 1,
				},
				ProjectCards: api.ProjectCards{
					Nodes: []*api.ProjectInfo{
						{
							Project: api.ProjectV1ProjectName{
								Name: "ProjectCardName",
							},
							Column: api.ProjectV1ProjectColumn{
								Name: "ProjectCardColumn",
							},
						},
					},
					TotalCount: 1,
				},
				ProjectItems: api.ProjectItems{
					Nodes: []*api.ProjectV2Item{
						{
							ID: "projectItemID",
							Project: api.ProjectV2ItemProject{
								ID:    "projectID",
								Title: "V2 Project Title",
							},
							Status: api.ProjectV2ItemStatus{
								OptionID: "statusID",
								Name:     "STATUS",
							},
						},
					},
				},
				Milestone: &api.Milestone{
					Title: "MilestoneTitle",
				},
				ReactionGroups: api.ReactionGroups{
					{
						Content: "THUMBS_UP",
						Users: api.ReactionGroupUsers{
							TotalCount: 1,
						},
					},
				},
				CreatedAt: createdAt,
			},
			expect: PresentationIssue{
				Title:     "Title",
				Number:    123,
				CreatedAt: createdAt,
				Comments: api.Comments{
					Nodes: []api.Comment{
						{
							Author: api.CommentAuthor{
								Login: "monalisa",
							},
							Body:           "comment body 1",
							ReactionGroups: api.ReactionGroups{},
						},
					},
					TotalCount: 1,
				},
				State:          "OPEN",
				StateReason:    "",
				Reactions:      "1 \U0001f44d",
				Author:         "author",
				AssigneesList:  "assignee",
				LabelsList:     "bug",
				ProjectsList:   "ProjectCardName (ProjectCardColumn), V2 Project Title (STATUS)",
				MilestoneTitle: "MilestoneTitle",
				Body:           "",
				URL:            "github.com/OWNER/REPO/issues/123",
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			presentationIssue, err := MapApiIssueToPresentationIssue(tc.issue, nil)
			if err != nil {
				t.Fatal(err)
			}

			// These are here to aid development and debugging of the individual pieces
			assert.Equal(t, presentationIssue.Title, tc.expect.Title)
			assert.Equal(t, presentationIssue.Number, tc.expect.Number)
			assert.Equal(t, presentationIssue.CreatedAt, tc.expect.CreatedAt)
			assert.Equal(t, presentationIssue.Comments, tc.expect.Comments)
			assert.Equal(t, presentationIssue.State, tc.expect.State)
			assert.Equal(t, presentationIssue.Reactions, tc.expect.Reactions)
			assert.Equal(t, presentationIssue.Author, tc.expect.Author)
			assert.Equal(t, presentationIssue.AssigneesList, tc.expect.AssigneesList)
			assert.Equal(t, presentationIssue.LabelsList, tc.expect.LabelsList)
			assert.Equal(t, presentationIssue.ProjectsList, tc.expect.ProjectsList)
			assert.Equal(t, presentationIssue.MilestoneTitle, tc.expect.MilestoneTitle)
			assert.Equal(t, presentationIssue.Body, tc.expect.Body)
			assert.Equal(t, presentationIssue.URL, tc.expect.URL)

			// This tests the entire struct
			assert.Equal(t, presentationIssue, tc.expect)
		})
	}
}

func Test_stringifyAssignees(t *testing.T) {
	tests := map[string]struct {
		assignees api.Assignees
		expected  string
	}{
		"two assignees": {
			assignees: api.Assignees{
				Nodes: []api.GitHubUser{
					{Login: "monalisa"},
					{Login: "hubot"},
				},
				TotalCount: 2,
			},
			expected: "monalisa, hubot",
		},
		"no assignees": {
			assignees: api.Assignees{},
			expected:  "",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.expected, stringifyAssignees(tc.assignees))
		})
	}
}

func Test_stringifyAndColorizeLabels(t *testing.T) {
	tests := map[string]struct {
		labels               api.Labels
		isColorSchemeEnabled bool
		expected             string
	}{
		"no labels": {
			labels:   api.Labels{},
			expected: "",
		},
		"single label no colorScheme": {
			labels: api.Labels{
				Nodes: []api.IssueLabel{
					{
						Name:  "bug",
						Color: "fc0303",
					},
				},
			},
			isColorSchemeEnabled: false,
			expected:             "bug",
		},
		"single label with colorScheme": {
			labels: api.Labels{
				Nodes: []api.IssueLabel{
					{
						Name:  "bug",
						Color: "fc0303",
					},
				},
			},
			isColorSchemeEnabled: true,
			expected:             "\033[38;2;252;3;3mbug\033[0m",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			ios.SetStdoutTTY(true)
			ios.SetStdinTTY(true)
			ios.SetStderrTTY(true)
			ios.SetColorEnabled(tc.isColorSchemeEnabled)

			assert.Equal(t, stringifyAndColorizeLabels(tc.labels, ios.ColorScheme()), tc.expected)
		})
	}
}

func Test_stringifyProjects(t *testing.T) {
	tests := map[string]struct {
		projectCards api.ProjectCards
		projectItems api.ProjectItems
		expected     string
	}{
		"no projects": {
			projectCards: api.ProjectCards{
				Nodes:      []*api.ProjectInfo{},
				TotalCount: 0,
			},
			projectItems: api.ProjectItems{
				Nodes: []*api.ProjectV2Item{},
			},
			expected: "",
		},
		"two v1 projects and no v2 projects": {
			projectCards: api.ProjectCards{
				Nodes: []*api.ProjectInfo{
					{Project: api.ProjectV1ProjectName{Name: "Project 1"}, Column: api.ProjectV1ProjectColumn{Name: "Column 1"}},
					{Project: api.ProjectV1ProjectName{Name: "Project 2"}, Column: api.ProjectV1ProjectColumn{Name: "Column 2"}},
				},
				TotalCount: 2,
			},
			projectItems: api.ProjectItems{
				Nodes: []*api.ProjectV2Item{},
			},
			expected: "Project 1 (Column 1), Project 2 (Column 2)",
		},
		"two v1 projects without columns and no v2 projects": {
			projectCards: api.ProjectCards{
				Nodes: []*api.ProjectInfo{
					{Project: api.ProjectV1ProjectName{Name: "Project 1"}, Column: api.ProjectV1ProjectColumn{Name: ""}},
					{Project: api.ProjectV1ProjectName{Name: "Project 2"}, Column: api.ProjectV1ProjectColumn{Name: ""}},
				},
				TotalCount: 2,
			},
			projectItems: api.ProjectItems{
				Nodes: []*api.ProjectV2Item{},
			},
			expected: "Project 1 (Awaiting triage), Project 2 (Awaiting triage)",
		},
		"no v1 projects and 2 v2 projects": {
			projectCards: api.ProjectCards{
				Nodes:      []*api.ProjectInfo{},
				TotalCount: 0,
			},
			projectItems: api.ProjectItems{
				Nodes: []*api.ProjectV2Item{
					{
						ID: "projectItemID",
						Project: api.ProjectV2ItemProject{
							ID:    "projectID",
							Title: "V2 Project Title",
						},
						Status: api.ProjectV2ItemStatus{
							OptionID: "statusID",
							Name:     "STATUS",
						},
					},
					{
						ID: "projectItemID2",
						Project: api.ProjectV2ItemProject{
							ID:    "projectID2",
							Title: "V2 Project Title 2",
						},
						Status: api.ProjectV2ItemStatus{
							OptionID: "statusID2",
							Name:     "",
						},
					},
				},
			},
			expected: "V2 Project Title (STATUS), V2 Project Title 2 (Backlog)",
		},
		"1 v1 project and 1 v2 project": {
			projectCards: api.ProjectCards{
				Nodes: []*api.ProjectInfo{
					{Project: api.ProjectV1ProjectName{Name: "V1 Project Name"}, Column: api.ProjectV1ProjectColumn{Name: "COLUMN"}},
				},
				TotalCount: 1,
			},
			projectItems: api.ProjectItems{
				Nodes: []*api.ProjectV2Item{
					{
						ID: "projectItemID",
						Project: api.ProjectV2ItemProject{
							ID:    "projectID",
							Title: "V2 Project Title",
						},
						Status: api.ProjectV2ItemStatus{
							OptionID: "statusID",
							Name:     "STATUS",
						},
					},
				},
			},
			expected: "V1 Project Name (COLUMN), V2 Project Title (STATUS)",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, stringifyProjects(tc.projectCards, tc.projectItems), tc.expected)
		})
	}
}

func Test_sortAlphabeticallyIgnoreCase(t *testing.T) {
	tests := map[string]struct {
		labels   api.Labels
		expected api.Labels
	}{
		"no repeat labels": {
			labels: api.Labels{
				Nodes: []api.IssueLabel{
					{Name: "c"},
					{Name: "B"},
					{Name: "a"},
				},
			},
			expected: api.Labels{
				Nodes: []api.IssueLabel{
					{Name: "a"},
					{Name: "B"},
					{Name: "c"},
				},
			},
		},
		"repeat labels case insensitive": {
			labels: api.Labels{
				Nodes: []api.IssueLabel{
					{Name: "c"},
					{Name: "B"},
					{Name: "C"},
				},
			},
			expected: api.Labels{
				Nodes: []api.IssueLabel{
					{Name: "B"},
					{Name: "c"},
					{Name: "C"},
				},
			},
		},
		"no labels": {
			labels: api.Labels{
				Nodes: []api.IssueLabel{},
			},
			expected: api.Labels{
				Nodes: []api.IssueLabel{},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.expected, sortAlphabeticallyIgnoreCase(tc.labels))
		})
	}
}
