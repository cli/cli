package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetAssigneeListString(t *testing.T) {
	tests := map[string]struct {
		assignees []GitHubUser
		expected  string
	}{
		"two assignees": {
			assignees: []GitHubUser{
				{Login: "monalisa"},
				{Login: "hubot"},
			},
			expected: "monalisa, hubot",
		},
		"no assignees": {
			assignees: []GitHubUser{},
			expected:  "",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			issue := &Issue{
				Assignees: Assignees{
					Nodes: tc.assignees,
				},
			}
			assert.Equal(t, tc.expected, issue.GetAssigneeListString())
		})
	}
}

func TestGetProjectListString(t *testing.T) {
	tests := map[string]struct {
		projectCards ProjectCards
		projectItems ProjectItems
		expected     string
	}{
		"no projects": {
			projectCards: ProjectCards{
				Nodes:      []*ProjectInfo{},
				TotalCount: 0,
			},
			projectItems: ProjectItems{
				Nodes: []*ProjectV2Item{},
			},
			expected: "",
		},
		"two v1 projects and no v2 projects": {
			projectCards: ProjectCards{
				Nodes: []*ProjectInfo{
					{Project: ProjectV1ProjectName{Name: "Project 1"}, Column: ProjectV1ProjectColumn{Name: "Column 1"}},
					{Project: ProjectV1ProjectName{Name: "Project 2"}, Column: ProjectV1ProjectColumn{Name: "Column 2"}},
				},
				TotalCount: 2,
			},
			projectItems: ProjectItems{
				Nodes: []*ProjectV2Item{},
			},
			expected: "Project 1 (Column 1), Project 2 (Column 2)",
		},
		"two v1 projects without columns and no v2 projects": {
			projectCards: ProjectCards{
				Nodes: []*ProjectInfo{
					{Project: ProjectV1ProjectName{Name: "Project 1"}, Column: ProjectV1ProjectColumn{Name: ""}},
					{Project: ProjectV1ProjectName{Name: "Project 2"}, Column: ProjectV1ProjectColumn{Name: ""}},
				},
				TotalCount: 2,
			},
			projectItems: ProjectItems{
				Nodes: []*ProjectV2Item{},
			},
			expected: "Project 1 (Awaiting triage), Project 2 (Awaiting triage)",
		},
		"no v1 projects and 2 v2 projects": {
			projectCards: ProjectCards{
				Nodes:      []*ProjectInfo{},
				TotalCount: 0,
			},
			projectItems: ProjectItems{
				Nodes: []*ProjectV2Item{},
			},
			expected: "",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			issue := &Issue{
				ProjectCards: tc.projectCards,
				ProjectItems: tc.projectItems,
			}
			assert.Equal(t, tc.expected, issue.GetProjectListString())
		})
	}
}

func Test_Labels_SortAlphabeticallyIgnoreCase(t *testing.T) {
	tests := map[string]struct {
		labels   Labels
		expected Labels
	}{
		"no repeat labels": {
			labels: Labels{
				Nodes: []IssueLabel{
					{Name: "c"},
					{Name: "B"},
					{Name: "a"},
				},
			},
			expected: Labels{
				Nodes: []IssueLabel{
					{Name: "a"},
					{Name: "B"},
					{Name: "c"},
				},
			},
		},
		"repeat labels case insensitive": {
			labels: Labels{
				Nodes: []IssueLabel{
					{Name: "c"},
					{Name: "B"},
					{Name: "C"},
				},
			},
			expected: Labels{
				Nodes: []IssueLabel{
					{Name: "B"},
					{Name: "c"},
					{Name: "C"},
				},
			},
		},
		"no labels": {
			labels: Labels{
				Nodes: []IssueLabel{},
			},
			expected: Labels{
				Nodes: []IssueLabel{},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tc.labels.SortAlphabeticallyIgnoreCase()
			assert.Equal(t, tc.expected, tc.labels)
		})
	}
}
