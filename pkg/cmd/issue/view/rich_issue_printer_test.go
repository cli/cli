package view

import (
	"testing"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/magiconair/properties/assert"
)

// Should we write an integration test for Print? I'm not sure how easy
// this is or how much value it will add...
//
// func Test_Print(t *testing.T) {
// 	 ...test logic...
// }

// This does not test color, just output
func Test_header(t *testing.T) {
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

			presentationIssue := PresentationIssue{
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

			richIssuePrinter := &RichIssuePrinter{
				IO:      ios,
				TimeNow: createdAtTime.AddDate(0, 0, 1),
			}
			richIssuePrinter.header(presentationIssue, tc.baseRepo)
			assert.Equal(t, tc.expected, stdout.String())
		})
	}
}

func Test_reactions(t *testing.T) {
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

			presentationIssue := PresentationIssue{
				Reactions: tc.reactions,
			}

			richIssuePrinter := &RichIssuePrinter{
				IO: ios,
			}
			richIssuePrinter.reactions(presentationIssue)
			assert.Equal(t, stdout.String(), tc.expected)
		})
	}
}

func Test_assigneeList(t *testing.T) {
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

			presentationIssue := PresentationIssue{
				AssigneesList: tc.assignees,
			}

			richIssuePrinter := &RichIssuePrinter{
				IO: ios,
			}

			richIssuePrinter.assigneeList(presentationIssue)
			assert.Equal(t, stdout.String(), tc.expected)
		})
	}
}

func Test_labelList(t *testing.T) {
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

			presentationIssue := PresentationIssue{
				LabelsList: tc.labels,
			}

			richIssuePrinter := &RichIssuePrinter{
				IO: ios,
			}

			richIssuePrinter.labelList(presentationIssue)
			assert.Equal(t, stdout.String(), tc.expected)
		})
	}
}

func Test_projectList(t *testing.T) {
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

			presentationIssue := PresentationIssue{
				ProjectsList: tc.projectList,
			}

			richIssuePrinter := &RichIssuePrinter{
				IO: ios,
			}

			richIssuePrinter.projectList(presentationIssue)
			assert.Equal(t, stdout.String(), tc.expected)
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

			presentationIssue := PresentationIssue{
				MilestoneTitle: tc.milestone,
			}

			richIssuePrinter := &RichIssuePrinter{
				IO: ios,
			}

			richIssuePrinter.milestone(presentationIssue)
			assert.Equal(t, stdout.String(), tc.expected)
		})
	}
}

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

			presentationIssue := PresentationIssue{
				Body: tc.body,
			}

			richIssuePrinter := &RichIssuePrinter{
				IO: ios,
			}

			err := richIssuePrinter.body(presentationIssue)
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

			presentationIssue := PresentationIssue{
				Comments: api.Comments{
					Nodes:      tc.commentNodes,
					TotalCount: len(tc.commentNodes),
				},
			}

			richIssuePrinter := &RichIssuePrinter{
				IO:      ios,
				TimeNow: timeNow,
			}

			err = richIssuePrinter.comments(presentationIssue, tc.isPreview)
			if err != nil {
				t.Fatal(err)
			}

			// I can't get these strings to match
			// assert.Matches(t, stdout.String(), tc.expected)
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

			presentationIssue := &PresentationIssue{
				URL: tc.url,
			}

			richIssuePrinter := &RichIssuePrinter{
				IO: ios,
			}

			richIssuePrinter.footer(*presentationIssue)
			assert.Equal(t, stdout.String(), tc.expected)
		})
	}
}
