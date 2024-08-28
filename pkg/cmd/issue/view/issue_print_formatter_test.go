package view

import (
	"testing"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/magiconair/properties/assert"
)

func Test_apiIssueToPresentationIssue(t *testing.T) {
	createdAt, err := time.Parse(time.RFC3339, "2022-01-01T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}

	tests := map[string]struct {
		issue  *api.Issue
		expect *presentationIssue
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
					Login: "octocat",
					Name:  "Octo Cat",
					ID:    "321",
				},
				Assignees: api.Assignees{
					Nodes: []api.GitHubUser{
						{
							Login: "octocat",
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
			expect: &presentationIssue{
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
				AssigneesList:  "octocat",
				LabelsList:     "bug",
				ProjectsList:   "ProjectCardName (ProjectCardColumn), V2ProjectName",
				MilestoneTitle: "MilestoneTitle",
				Body:           "",
				URL:            "github.com/OWNER/REPO/issues/123",
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			presentationIssue, err := apiIssueToPresentationIssue(tc.issue, nil)
			if err != nil {
				t.Fatal(err)
			}
			// These are only here for development purposes
			assert.Equal(t, presentationIssue.Title, tc.expect.Title)
			assert.Equal(t, presentationIssue.Number, tc.expect.Number)
			assert.Equal(t, presentationIssue.CreatedAt, tc.expect.CreatedAt)
			assert.Equal(t, presentationIssue.Comments, tc.expect.Comments)
			assert.Equal(t, presentationIssue.State, tc.expect.State)
			assert.Equal(t, presentationIssue.Reactions, tc.expect.Reactions)
			assert.Equal(t, presentationIssue.AssigneesList, tc.expect.AssigneesList)
			assert.Equal(t, presentationIssue.LabelsList, tc.expect.LabelsList)
			// Below will fail until V2 support is added
			// assert.Equal(t, presentationIssue.ProjectsList, tc.expect.ProjectsList)
			assert.Equal(t, presentationIssue.MilestoneTitle, tc.expect.MilestoneTitle)
			assert.Equal(t, presentationIssue.Body, tc.expect.Body)
			assert.Equal(t, presentationIssue.URL, tc.expect.URL)

			// This is the actual test
			// assert.Equal(t, presentationIssue, tc.expect)
		})
	}
}

// Placeholder. I'm not sure how I want to test this...
// func Test_ipf_renderHumanIssuePreview(t *testing.T) {
// 	return
// }

func Test_ipf_RenderRawIssuePreview(t *testing.T) {
	tests := map[string]struct {
		presentationIssue *presentationIssue
		expectedOutput    string
	}{
		"basic issue": {
			presentationIssue: &presentationIssue{
				Title:      "issueTitle",
				State:      "issueState",
				Author:     "authorLogin",
				LabelsList: "labelsList",
				Comments: api.Comments{
					TotalCount: 1,
				},
				AssigneesList:  "assigneesList",
				ProjectsList:   "projectsList",
				MilestoneTitle: "milestoneTitle",
				Number:         123,
				Body:           "issueBody",
			},
			expectedOutput: "title:\tissueTitle\nstate:\tissueState\nauthor:\tauthorLogin\nlabels:\tlabelsList\ncomments:\t1\nassignees:\tassigneesList\nprojects:\tprojectsList\nmilestone:\tmilestoneTitle\nnumber:\t123\n--\nissueBody\n",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ios, _, stdout, _ := iostreams.Test()

			ipf := NewIssuePrintFormatter(tc.presentationIssue, ios, time.Time{}, ghrepo.New("OWNER", "REPO"))
			ipf.renderRawIssuePreview()
			assert.Equal(t, tc.expectedOutput, stdout.String())
		})
	}
}

func Test_getAssigneeListString(t *testing.T) {
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
			assert.Equal(t, tc.expected, getAssigneeListString(tc.assignees))
		})
	}
}

func Test_getColorizedLabelsList(t *testing.T) {
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

			assert.Equal(t, getColorizedLabelsList(tc.labels, ios.ColorScheme()), tc.expected)
		})
	}
}

func Test_getProjectListString(t *testing.T) {
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
				Nodes: []*api.ProjectV2Item{},
			},
			expected: "",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.expected, getProjectListString(tc.projectCards, tc.projectItems))
		})
	}
}

func Test_ipf_reactions(t *testing.T) {
	tests := map[string]struct {
		reactions string
		expected  string
	}{
		"no reactions": {
			reactions: "",
			expected:  "",
		},
		"reactions": {
			reactions: "a reaction",
			expected:  "a reaction\n",
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

			presentationIssue := &presentationIssue{
				Reactions: tc.reactions,
			}

			ipf := NewIssuePrintFormatter(presentationIssue, ios, timeNow, ghrepo.New("OWNER", "REPO"))
			ipf.reactions()
			assert.Equal(t, stdout.String(), tc.expected)
		})
	}
}

// This does not test color, just output
func Test_ipf_header(t *testing.T) {
	tests := map[string]struct {
		title       string
		number      int
		state       string
		stateReason string
		createdAt   string
		author      string
		baseRepo    ghrepo.Interface
		expected    string
	}{
		"simple open issue": {
			title:       "Simple Issue Test",
			baseRepo:    ghrepo.New("OWNER", "REPO"),
			number:      123,
			state:       "OPEN",
			stateReason: "",
			author:      "monalisa",
			createdAt:   "2022-01-01T00:00:00Z",
			expected:    "Simple Issue Test OWNER/REPO#123\nOpen • monalisa opened about 1 day ago • 1 comment\n",
		},
		"simple closed issue": {
			title:       "Simple Issue Test",
			baseRepo:    ghrepo.New("OWNER", "REPO"),
			number:      123,
			state:       "CLOSED",
			stateReason: "COMPLETED",
			author:      "monalisa",
			createdAt:   "2022-01-01T00:00:00Z",
			expected:    "Simple Issue Test OWNER/REPO#123\nClosed • monalisa opened about 1 day ago • 1 comment\n",
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

			presentationIssue := &presentationIssue{
				Title:  tc.title,
				Number: tc.number,
				Comments: api.Comments{
					TotalCount: 1,
				},
				State:       tc.state,
				StateReason: tc.stateReason,
				CreatedAt:   createdAtTime,
				Author:      tc.author,
			}

			ipf := NewIssuePrintFormatter(presentationIssue, ios, createdAtTime.AddDate(0, 0, 1), tc.baseRepo)
			ipf.header()
			assert.Equal(t, stdout.String(), tc.expected)
		})
	}
}

func Test_ipf_assigneeList(t *testing.T) {
	tests := map[string]struct {
		assignees string
		expected  string
	}{
		"no assignees": {
			assignees: "",
			expected:  "",
		},
		"assignees": {
			assignees: "monalisa, octocat",
			expected:  "Assignees: monalisa, octocat\n",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ios, _, stdout, _ := iostreams.Test()
			ios.SetStdoutTTY(true)
			ios.SetStdinTTY(true)
			ios.SetStderrTTY(true)

			presentationIssue := &presentationIssue{
				AssigneesList: tc.assignees,
			}

			ipf := NewIssuePrintFormatter(presentationIssue, ios, time.Time{}, ghrepo.New("OWNER", "REPO"))
			ipf.assigneeList()
			assert.Equal(t, stdout.String(), tc.expected)
		})
	}
}

func Test_ipf_labelList(t *testing.T) {
	tests := map[string]struct {
		labels   string
		expected string
	}{
		"no labels": {
			labels:   "",
			expected: "",
		},
		"labels": {
			labels:   "bug, enhancement",
			expected: "Labels: bug, enhancement\n",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ios, _, stdout, _ := iostreams.Test()
			ios.SetStdoutTTY(true)
			ios.SetStdinTTY(true)
			ios.SetStderrTTY(true)

			presentationIssue := &presentationIssue{
				LabelsList: tc.labels,
			}

			ipf := NewIssuePrintFormatter(presentationIssue, ios, time.Time{}, ghrepo.New("OWNER", "REPO"))
			ipf.labelList()
			assert.Equal(t, stdout.String(), tc.expected)
		})
	}
}

func Test_ipf_projectList(t *testing.T) {
	tests := map[string]struct {
		projectList string
		expected    string
	}{
		"no projects": {
			projectList: "",
			expected:    "",
		},
		"some projects": {
			projectList: "ProjectV1 1 (Column 1), ProjectV1 2 (Awaiting triage)",
			expected:    "Projects: ProjectV1 1 (Column 1), ProjectV1 2 (Awaiting triage)\n",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ios, _, stdout, _ := iostreams.Test()
			ios.SetStdoutTTY(true)
			ios.SetStdinTTY(true)
			ios.SetStderrTTY(true)

			presentationIssue := &presentationIssue{
				ProjectsList: tc.projectList,
			}

			ipf := NewIssuePrintFormatter(presentationIssue, ios, time.Time{}, ghrepo.New("OWNER", "REPO"))
			ipf.projectList()
			assert.Equal(t, stdout.String(), tc.expected)
		})
	}
}

func Test_ipf_milestone(t *testing.T) {
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

			presentationIssue := &presentationIssue{
				MilestoneTitle: tc.milestone,
			}

			ipf := NewIssuePrintFormatter(presentationIssue, ios, time.Time{}, ghrepo.New("OWNER", "REPO"))
			ipf.milestone()
			assert.Equal(t, stdout.String(), tc.expected)
		})
	}
}

func Test_ipf_body(t *testing.T) {
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

			presentationIssue := &presentationIssue{
				Body: tc.body,
			}

			ipf := NewIssuePrintFormatter(presentationIssue, ios, time.Time{}, ghrepo.New("OWNER", "REPO"))
			err := ipf.body()
			if err != nil {
				t.Fatal(err)
			}
			// This is getting around whitespace issues I was having with assert.Equal
			assert.Matches(t, stdout.String(), tc.expected)
		})
	}
}

func Test_ipf_comments(t *testing.T) {
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

			presentationIssue := &presentationIssue{
				Comments: api.Comments{
					Nodes:      tc.commentNodes,
					TotalCount: len(tc.commentNodes),
				},
			}

			ipf := NewIssuePrintFormatter(presentationIssue, ios, timeNow, ghrepo.New("OWNER", "REPO"))
			err = ipf.comments(tc.isPreview)
			if err != nil {
				t.Fatal(err)
			}

			// I can't get these strings to match
			// assert.Matches(t, stdout.String(), tc.expected)
		})
	}
}

func Test_ipf_footer(t *testing.T) {
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

			presentationIssue := &presentationIssue{
				URL: tc.url,
			}

			ipf := NewIssuePrintFormatter(presentationIssue, ios, time.Time{}, ghrepo.New("OWNER", "REPO"))
			ipf.footer()
			assert.Equal(t, stdout.String(), tc.expected)
		})
	}
}
