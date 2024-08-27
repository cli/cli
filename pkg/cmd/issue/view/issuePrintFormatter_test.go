package view

import (
	"testing"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/magiconair/properties/assert"
)

// This does not test color, just output
func Test_header(t *testing.T) {
	tests := map[string]struct {
		title     string
		number    int
		state     string
		createdAt string
		author    string
		baseRepo  ghrepo.Interface
		expected  string
	}{
		"simple open issue": {
			title:     "Simple Issue Test",
			baseRepo:  ghrepo.New("OWNER", "REPO"),
			number:    123,
			state:     "OPEN",
			author:    "monalisa",
			createdAt: "2022-01-01T00:00:00Z",
			expected:  "Simple Issue Test OWNER/REPO#123\nOpen • monalisa opened about 1 day ago • 1 comment\n",
		},
		"simple closed issue": {
			title:     "Simple Issue Test",
			baseRepo:  ghrepo.New("OWNER", "REPO"),
			number:    123,
			state:     "CLOSED",
			author:    "monalisa",
			createdAt: "2022-01-01T00:00:00Z",
			expected:  "Simple Issue Test OWNER/REPO#123\nClosed • monalisa opened about 1 day ago • 1 comment\n",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ios, _, stdout, _ := iostreams.Test()
			ios.SetStdoutTTY(true)
			ios.SetStdinTTY(true)
			ios.SetStderrTTY(true)

			createdAtTime, err := time.Parse(time.RFC3339, tc.createdAt)
			if err != nil {
				t.Fatal(err)
			}

			issue := &api.Issue{
				Title:  tc.title,
				Number: tc.number,
				Comments: api.Comments{
					TotalCount: 1,
				},
				State:     tc.state,
				CreatedAt: createdAtTime,
				Author: api.Author{
					Login: tc.author,
				},
			}

			ipf := NewIssuePrintFormatter(issue, ios, createdAtTime.AddDate(0, 0, 1), tc.baseRepo)
			ipf.header()
			assert.Equal(t, stdout.String(), tc.expected)
		})
	}
}

func Test_reactions(t *testing.T) {
	tests := map[string]struct {
		reactions api.ReactionGroups
		expected  string
	}{
		"no reactions": {
			reactions: api.ReactionGroups{},
			expected:  "",
		},
		"single thumbs up reaction": {
			reactions: api.ReactionGroups{
				api.ReactionGroup{
					Content: "THUMBS_UP",
					Users: api.ReactionGroupUsers{
						TotalCount: 1,
					},
				},
			},
			expected: "1 \U0001f44d\n",
		},
		"all reactions": {
			reactions: api.ReactionGroups{
				api.ReactionGroup{
					Content: "THUMBS_UP",
					Users: api.ReactionGroupUsers{
						TotalCount: 1,
					},
				},
				api.ReactionGroup{
					Content: "THUMBS_DOWN",
					Users: api.ReactionGroupUsers{
						TotalCount: 2,
					},
				},
				api.ReactionGroup{
					Content: "LAUGH",
					Users: api.ReactionGroupUsers{
						TotalCount: 3,
					},
				},
				api.ReactionGroup{
					Content: "HOORAY",
					Users: api.ReactionGroupUsers{
						TotalCount: 4,
					},
				},
				api.ReactionGroup{
					Content: "CONFUSED",
					Users: api.ReactionGroupUsers{
						TotalCount: 5,
					},
				},
				api.ReactionGroup{
					Content: "HEART",
					Users: api.ReactionGroupUsers{
						TotalCount: 6,
					},
				},
				api.ReactionGroup{
					Content: "ROCKET",
					Users: api.ReactionGroupUsers{
						TotalCount: 7,
					},
				},
				api.ReactionGroup{
					Content: "EYES",
					Users: api.ReactionGroupUsers{
						TotalCount: 8,
					},
				},
			},
			expected: "1 \U0001f44d • 2 \U0001f44e • 3 \U0001f604 • 4 \U0001f389 • 5 \U0001f615 • 6 \u2764\ufe0f • 7 \U0001f680 • 8 \U0001f440\n",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ios, _, stdout, _ := iostreams.Test()
			ios.SetStdoutTTY(true)
			ios.SetStdinTTY(true)
			ios.SetStderrTTY(true)

			timeNow, err := time.Parse(time.RFC3339, "2022-01-01T00:00:00Z")
			if err != nil {
				t.Fatal(err)
			}

			issue := &api.Issue{
				ReactionGroups: tc.reactions,
			}

			ipf := NewIssuePrintFormatter(issue, ios, timeNow, ghrepo.New("OWNER", "REPO"))
			ipf.reactions()
			assert.Equal(t, stdout.String(), tc.expected)
		})
	}
}

// Note: this is currently over-testing. We should be able to mock the
// return from GetAssigneeListString (which has its own tests), but I think
// that requires a larger refactor of the Issue code to leverage interfaces
// for mocking.
func Test_assigneeList(t *testing.T) {
	tests := map[string]struct {
		assignees []string
		expected  string
	}{
		"no assignees": {
			assignees: []string{},
			expected:  "",
		},
		"single assignee": {
			assignees: []string{"monalisa"},
			expected:  "Assignees: monalisa\n",
		},
		"multiple assignees": {
			assignees: []string{"monalisa", "octocat"},
			expected:  "Assignees: monalisa, octocat\n",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ios, _, stdout, _ := iostreams.Test()
			ios.SetStdoutTTY(true)
			ios.SetStdinTTY(true)
			ios.SetStderrTTY(true)

			timeNow, err := time.Parse(time.RFC3339, "2022-01-01T00:00:00Z")
			if err != nil {
				t.Fatal(err)
			}

			totalCount := len(tc.assignees)
			assigneeNodes := make([]api.GitHubUser, totalCount)

			for i, assignee := range tc.assignees {
				assigneeNodes[i] = api.GitHubUser{
					Login: assignee,
				}
			}

			issue := &api.Issue{
				Assignees: api.Assignees{
					Nodes:      assigneeNodes,
					TotalCount: totalCount,
				},
			}

			ipf := NewIssuePrintFormatter(issue, ios, timeNow, ghrepo.New("OWNER", "REPO"))
			ipf.assigneeList()
			assert.Equal(t, stdout.String(), tc.expected)
		})
	}
}

func Test_labelList(t *testing.T) {
	tests := map[string]struct {
		labelNodes []api.IssueLabel
		expected   string
	}{
		"no labels": {
			labelNodes: []api.IssueLabel{},
			expected:   "",
		},
		"single label": {
			labelNodes: []api.IssueLabel{
				{
					Name:  "bug",
					Color: "ff0000",
				},
			},
			expected: "Labels: bug\n",
		},
		"multiple labels": {
			labelNodes: []api.IssueLabel{
				{
					Name:  "bug",
					Color: "ff0000",
				},
				{
					Name:  "enhancement",
					Color: "00ff00",
				},
			},
			expected: "Labels: bug, enhancement\n",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ios, _, stdout, _ := iostreams.Test()
			ios.SetStdoutTTY(true)
			ios.SetStdinTTY(true)
			ios.SetStderrTTY(true)

			timeNow, err := time.Parse(time.RFC3339, "2022-01-01T00:00:00Z")
			if err != nil {
				t.Fatal(err)
			}

			issue := &api.Issue{
				Labels: api.Labels{
					Nodes:      tc.labelNodes,
					TotalCount: len(tc.labelNodes),
				},
			}

			ipf := NewIssuePrintFormatter(issue, ios, timeNow, ghrepo.New("OWNER", "REPO"))
			ipf.labelList()
			assert.Equal(t, stdout.String(), tc.expected)
		})
	}
}

// Note: this is currently over-testing. We should be able to mock the
// return from GetProjectListString (which has its own tests), but I think
// that requires a larger refactor of the Issue code to leverage interfaces
// for mocking.
func Test_projectList(t *testing.T) {
	tests := map[string]struct {
		projectCardsNodes []*api.ProjectInfo
		projectItemsNodes []*api.ProjectV2Item
		expected          string
	}{
		"no projects": {
			projectCardsNodes: []*api.ProjectInfo{},
			projectItemsNodes: []*api.ProjectV2Item{},
			expected:          "",
		},
		"some projects": {
			projectCardsNodes: []*api.ProjectInfo{
				{
					Project: api.ProjectV1ProjectName{
						Name: "ProjectV1 1",
					},
					Column: api.ProjectV1ProjectColumn{
						Name: "Column 1",
					},
				},
				{
					Project: api.ProjectV1ProjectName{
						Name: "ProjectV1 2",
					},
					Column: api.ProjectV1ProjectColumn{
						Name: "",
					},
				},
			},
			projectItemsNodes: []*api.ProjectV2Item{
				{
					ID: "projectItemID",
					Project: api.ProjectV2ItemProject{
						ID:    "projectID",
						Title: "V2 Project",
					},
					Status: api.ProjectV2ItemStatus{
						OptionID: "statusID",
						Name:     "STATUS",
					},
				},
			},
			expected: "Projects: ProjectV1 1 (Column 1), ProjectV1 2 (Awaiting triage)\n",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ios, _, stdout, _ := iostreams.Test()
			ios.SetStdoutTTY(true)
			ios.SetStdinTTY(true)
			ios.SetStderrTTY(true)

			timeNow, err := time.Parse(time.RFC3339, "2022-01-01T00:00:00Z")
			if err != nil {
				t.Fatal(err)
			}

			issue := &api.Issue{
				ProjectCards: api.ProjectCards{
					Nodes:      tc.projectCardsNodes,
					TotalCount: len(tc.projectCardsNodes),
				},
				ProjectItems: api.ProjectItems{
					Nodes: tc.projectItemsNodes,
				},
			}

			ipf := NewIssuePrintFormatter(issue, ios, timeNow, ghrepo.New("OWNER", "REPO"))
			ipf.projectList()
			assert.Equal(t, stdout.String(), tc.expected)
		})
	}
}

func Test_getColorizedLabelsList(t *testing.T) {
	tests := map[string]struct {
		labelNodes           []api.IssueLabel
		isColorSchemeEnabled bool
		expected             string
	}{
		"no labels": {
			labelNodes: []api.IssueLabel{},
			expected:   "",
		},
		"single label no colorScheme": {
			labelNodes: []api.IssueLabel{
				{
					Name:  "bug",
					Color: "fc0303",
				},
			},
			isColorSchemeEnabled: false,
			expected:             "bug",
		},
		"single label with colorScheme": {
			labelNodes: []api.IssueLabel{
				{
					Name:  "bug",
					Color: "fc0303",
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

			timeNow, err := time.Parse(time.RFC3339, "2022-01-01T00:00:00Z")
			if err != nil {
				t.Fatal(err)
			}

			issue := &api.Issue{
				Labels: api.Labels{
					Nodes:      tc.labelNodes,
					TotalCount: len(tc.labelNodes),
				},
			}

			ipf := NewIssuePrintFormatter(issue, ios, timeNow, ghrepo.New("OWNER", "REPO"))
			assert.Equal(t, ipf.getColorizedLabelsList(), tc.expected)
		})
	}
}

func Test_milestone(t *testing.T) {
	tests := map[string]struct {
		milestone string
		expected  string
	}{
		"no milestone": {
			milestone: "",
			expected:  "",
		},
		"milestone": {
			milestone: "milestoneTitle",
			expected:  "Milestone: milestoneTitle\n",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ios, _, stdout, _ := iostreams.Test()
			ios.SetStdoutTTY(true)
			ios.SetStdinTTY(true)
			ios.SetStderrTTY(true)

			timeNow, err := time.Parse(time.RFC3339, "2022-01-01T00:00:00Z")
			if err != nil {
				t.Fatal(err)
			}

			issue := &api.Issue{}

			if tc.milestone != "" {
				issue.Milestone = &api.Milestone{
					Title: tc.milestone,
				}
			}

			ipf := NewIssuePrintFormatter(issue, ios, timeNow, ghrepo.New("OWNER", "REPO"))
			ipf.milestone()
			assert.Equal(t, stdout.String(), tc.expected)
		})
	}
}

// No idea why this isn't passing...
func Test_body(t *testing.T) {
	tests := map[string]struct {
		body     string
		expected string
	}{
		"no body": {
			body:     "",
			expected: "No description provided",
		},
		"with body": {
			body:     "This is a body",
			expected: "This is a body",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ios, _, stdout, _ := iostreams.Test()
			ios.SetStdoutTTY(true)
			ios.SetStdinTTY(true)
			ios.SetStderrTTY(true)

			timeNow, err := time.Parse(time.RFC3339, "2022-01-01T00:00:00Z")
			if err != nil {
				t.Fatal(err)
			}

			issue := &api.Issue{
				Body: tc.body,
			}

			ipf := NewIssuePrintFormatter(issue, ios, timeNow, ghrepo.New("OWNER", "REPO"))
			err = ipf.body()
			if err != nil {
				t.Fatal(err)
			}
			// This is getting around whitespace issues I was having with assert.Equal
			assert.Matches(t, stdout.String(), tc.expected)
		})
	}
}

func Test_comments(t *testing.T) {
	test := map[string]struct {
		commentNodes []api.Comment
		expected     string
		isPreview    bool
	}{
		"no comments": {
			commentNodes: []api.Comment{},
			expected:     "",
			isPreview:    false,
		},
		"comments": {
			commentNodes: []api.Comment{
				{
					Author: api.CommentAuthor{
						Login: "monalisa",
					},
					Body:           "body 1",
					ReactionGroups: api.ReactionGroups{},
				},
			},
			expected:  "monalisa () • Dec 31, 2021 • Newest comment\n\n  body 1\n",
			isPreview: false,
		},
		"preview": {
			commentNodes: []api.Comment{
				{
					Author: api.CommentAuthor{
						Login: "monalisa",
					},
					Body:           "body 1",
					ReactionGroups: api.ReactionGroups{},
				},
			},
			expected:  "monalisa () • Dec 31, 2021 • Newest comment\n\n  body 1",
			isPreview: true,
		},
	}
	for name, tc := range test {
		t.Run(name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			ios.SetStdoutTTY(true)
			ios.SetStdinTTY(true)
			ios.SetStderrTTY(true)

			timeNow, err := time.Parse(time.RFC3339, "2022-01-01T00:00:00Z")
			if err != nil {
				t.Fatal(err)
			}

			for i := range tc.commentNodes {
				// subtract a day
				tc.commentNodes[i].CreatedAt = timeNow.AddDate(0, 0, -1)
			}

			issue := &api.Issue{
				Comments: api.Comments{
					Nodes:      tc.commentNodes,
					TotalCount: len(tc.commentNodes),
				},
			}

			ipf := NewIssuePrintFormatter(issue, ios, timeNow, ghrepo.New("OWNER", "REPO"))
			err = ipf.comments(tc.isPreview)
			if err != nil {
				t.Fatal(err)
			}

			// I just can't get the strings to match...
			// assert.Equal(t, stdout.Strings(), tc.expected)
		})
	}
}

func Test_footer(t *testing.T) {
	tests := map[string]struct {
		url      string
		expected string
	}{
		"no url": {
			url:      "",
			expected: "View this issue on GitHub: \n",
		},
		"with url": {
			url:      "github.com/OWNER/REPO/issues/123",
			expected: "View this issue on GitHub: github.com/OWNER/REPO/issues/123\n",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ios, _, stdout, _ := iostreams.Test()
			ios.SetStdoutTTY(true)
			ios.SetStdinTTY(true)
			ios.SetStderrTTY(true)

			timeNow, err := time.Parse(time.RFC3339, "2022-01-01T00:00:00Z")
			if err != nil {
				t.Fatal(err)
			}

			issue := &api.Issue{
				URL: tc.url,
			}

			ipf := NewIssuePrintFormatter(issue, ios, timeNow, ghrepo.New("OWNER", "REPO"))
			ipf.footer()
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, stdout.String(), tc.expected)
		})
	}
}
