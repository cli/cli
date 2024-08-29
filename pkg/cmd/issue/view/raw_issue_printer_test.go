package view

import (
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/magiconair/properties/assert"
)

func TestRawIssuePrinting(t *testing.T) {
	tests := map[string]struct {
		comments          bool
		presentationIssue PresentationIssue
		expectedOutput    string
	}{
		"basic issue, no comments requested": {
			comments: false,
			presentationIssue: PresentationIssue{
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
		"basic issue, displays only comments when requested": {
			comments: true,
			presentationIssue: PresentationIssue{
				Comments: api.Comments{
					TotalCount: 1,
					Nodes: []api.Comment{
						{
							Author: api.CommentAuthor{
								Login: "test-author",
							},
							AuthorAssociation:   "member",
							Body:                "comment body",
							IncludesCreatedEdit: false,
						},
					},
				},
			},
			expectedOutput: "author:\ttest-author\nassociation:\tmember\nedited:\tfalse\nstatus:\tnone\n--\ncomment body\n--\n",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ios, _, stdout, _ := iostreams.Test()

			rip := &RawIssuePrinter{
				IO:       ios,
				Comments: tc.comments,
			}

			rip.Print(tc.presentationIssue, ghrepo.New("OWNER", "REPO"))
			assert.Equal(t, tc.expectedOutput, stdout.String())
		})
	}
}
