package edit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	fd "github.com/cli/cli/v2/internal/featuredetection"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/run"
	prShared "github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdEdit(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "my-body.md")
	err := os.WriteFile(tmpFile, []byte("a body from file"), 0600)
	require.NoError(t, err)

	tests := []struct {
		name             string
		input            string
		stdin            string
		output           EditOptions
		expectedBaseRepo ghrepo.Interface
		wantsErr         bool
	}{
		{
			name:     "no argument",
			input:    "",
			output:   EditOptions{},
			wantsErr: true,
		},
		{
			name:  "issue number argument",
			input: "23",
			output: EditOptions{
				IssueNumbers: []int{23},
				Interactive:  true,
			},
			wantsErr: false,
		},
		{
			name:  "title flag",
			input: "23 --title test",
			output: EditOptions{
				IssueNumbers: []int{23},
				Editable: prShared.Editable{
					Title: prShared.EditableString{
						Value:  "test",
						Edited: true,
					},
				},
			},
			wantsErr: false,
		},
		{
			name:  "body flag",
			input: "23 --body test",
			output: EditOptions{
				IssueNumbers: []int{23},
				Editable: prShared.Editable{
					Body: prShared.EditableString{
						Value:  "test",
						Edited: true,
					},
				},
			},
			wantsErr: false,
		},
		{
			name:  "body from stdin",
			input: "23 --body-file -",
			stdin: "this is on standard input",
			output: EditOptions{
				IssueNumbers: []int{23},
				Editable: prShared.Editable{
					Body: prShared.EditableString{
						Value:  "this is on standard input",
						Edited: true,
					},
				},
			},
			wantsErr: false,
		},
		{
			name:  "body from file",
			input: fmt.Sprintf("23 --body-file '%s'", tmpFile),
			output: EditOptions{
				IssueNumbers: []int{23},
				Editable: prShared.Editable{
					Body: prShared.EditableString{
						Value:  "a body from file",
						Edited: true,
					},
				},
			},
			wantsErr: false,
		},
		{
			name:     "both body and body-file flags",
			input:    "23 --body foo --body-file bar",
			wantsErr: true,
		},
		{
			name:  "add-assignee flag",
			input: "23 --add-assignee monalisa,hubot",
			output: EditOptions{
				IssueNumbers: []int{23},
				Editable: prShared.Editable{
					Assignees: prShared.EditableAssignees{
						EditableSlice: prShared.EditableSlice{
							Add:    []string{"monalisa", "hubot"},
							Edited: true,
						},
					},
				},
			},
			wantsErr: false,
		},
		{
			name:  "remove-assignee flag",
			input: "23 --remove-assignee monalisa,hubot",
			output: EditOptions{
				IssueNumbers: []int{23},
				Editable: prShared.Editable{
					Assignees: prShared.EditableAssignees{
						EditableSlice: prShared.EditableSlice{
							Remove: []string{"monalisa", "hubot"},
							Edited: true,
						},
					},
				},
			},
			wantsErr: false,
		},
		{
			name:  "add-label flag",
			input: "23 --add-label feature,TODO,bug",
			output: EditOptions{
				IssueNumbers: []int{23},
				Editable: prShared.Editable{
					Labels: prShared.EditableSlice{
						Add:    []string{"feature", "TODO", "bug"},
						Edited: true,
					},
				},
			},
			wantsErr: false,
		},
		{
			name:  "remove-label flag",
			input: "23 --remove-label feature,TODO,bug",
			output: EditOptions{
				IssueNumbers: []int{23},
				Editable: prShared.Editable{
					Labels: prShared.EditableSlice{
						Remove: []string{"feature", "TODO", "bug"},
						Edited: true,
					},
				},
			},
			wantsErr: false,
		},
		{
			name:  "add-project flag",
			input: "23 --add-project Cleanup,Roadmap",
			output: EditOptions{
				IssueNumbers: []int{23},
				Editable: prShared.Editable{
					Projects: prShared.EditableProjects{
						EditableSlice: prShared.EditableSlice{
							Add:    []string{"Cleanup", "Roadmap"},
							Edited: true,
						},
					},
				},
			},
			wantsErr: false,
		},
		{
			name:  "remove-project flag",
			input: "23 --remove-project Cleanup,Roadmap",
			output: EditOptions{
				IssueNumbers: []int{23},
				Editable: prShared.Editable{
					Projects: prShared.EditableProjects{
						EditableSlice: prShared.EditableSlice{
							Remove: []string{"Cleanup", "Roadmap"},
							Edited: true,
						},
					},
				},
			},
			wantsErr: false,
		},
		{
			name:  "milestone flag",
			input: "23 --milestone GA",
			output: EditOptions{
				IssueNumbers: []int{23},
				Editable: prShared.Editable{
					Milestone: prShared.EditableString{
						Value:  "GA",
						Edited: true,
					},
				},
			},
			wantsErr: false,
		},
		{
			name:  "remove-milestone flag",
			input: "23 --remove-milestone",
			output: EditOptions{
				IssueNumbers: []int{23},
				Editable: prShared.Editable{
					Milestone: prShared.EditableString{
						Value:  "",
						Edited: true,
					},
				},
			},
			wantsErr: false,
		},
		{
			name:     "both milestone and remove-milestone flags",
			input:    "23 --milestone foo --remove-milestone",
			wantsErr: true,
		},
		{
			name:  "add label to multiple issues",
			input: "23 34 --add-label bug",
			output: EditOptions{
				IssueNumbers: []int{23, 34},
				Editable: prShared.Editable{
					Labels: prShared.EditableSlice{
						Add:    []string{"bug"},
						Edited: true,
					},
				},
			},
			wantsErr: false,
		},
		{
			name: "argument is hash prefixed number",
			// Escaping is required here to avoid what I think is shellex treating it as a comment.
			input: "\\#23",
			output: EditOptions{
				IssueNumbers: []int{23},
				Interactive:  true,
			},
			wantsErr: false,
		},
		{
			name:  "argument is a URL",
			input: "https://example.com/cli/cli/issues/23",
			output: EditOptions{
				IssueNumbers: []int{23},
				Interactive:  true,
			},
			expectedBaseRepo: ghrepo.NewWithHost("cli", "cli", "example.com"),
			wantsErr:         false,
		},
		{
			name:     "URL arguments parse as different repos",
			input:    "https://github.com/cli/cli/issues/23 https://github.com/cli/go-gh/issues/23",
			wantsErr: true,
		},
		{
			name:     "interactive multiple issues",
			input:    "23 34",
			wantsErr: true,
		},
		{
			name:  "type flag",
			input: "23 --type Bug",
			output: EditOptions{
				IssueNumbers: []int{23},
				Editable: prShared.Editable{
					IssueType: prShared.EditableString{
						Value:  "Bug",
						Edited: true,
					},
				},
			},
		},
		{
			name:  "remove-type flag",
			input: "23 --remove-type",
			output: EditOptions{
				IssueNumbers:    []int{23},
				RemoveIssueType: true,
			},
		},
		{
			name:     "both type and remove-type flags",
			input:    "23 --type Bug --remove-type",
			wantsErr: true,
		},
		{
			name:  "parent flag",
			input: "23 --parent 100",
			output: EditOptions{
				IssueNumbers: []int{23},
				Parent:       "100",
			},
		},
		{
			name:  "remove-parent flag",
			input: "23 --remove-parent",
			output: EditOptions{
				IssueNumbers: []int{23},
				RemoveParent: true,
			},
		},
		{
			name:     "both parent and remove-parent flags",
			input:    "23 --parent 100 --remove-parent",
			wantsErr: true,
		},
		{
			name:  "add-sub-issue flag",
			input: "23 --add-sub-issue 123,124",
			output: EditOptions{
				IssueNumbers: []int{23},
				AddSubIssues: []string{"123", "124"},
			},
		},
		{
			name:     "add-sub-issue rejected with multiple issues",
			input:    "23 24 --add-sub-issue 123",
			wantsErr: true,
		},
		{
			name:  "remove-sub-issue flag",
			input: "23 --remove-sub-issue 50",
			output: EditOptions{
				IssueNumbers:    []int{23},
				RemoveSubIssues: []string{"50"},
			},
		},
		{
			name:  "add-blocked-by flag",
			input: "23 --add-blocked-by 200",
			output: EditOptions{
				IssueNumbers: []int{23},
				AddBlockedBy: []string{"200"},
			},
		},
		{
			name:  "remove-blocked-by flag",
			input: "23 --remove-blocked-by 201",
			output: EditOptions{
				IssueNumbers:    []int{23},
				RemoveBlockedBy: []string{"201"},
			},
		},
		{
			name:  "add-blocking flag",
			input: "23 --add-blocking 300,301",
			output: EditOptions{
				IssueNumbers: []int{23},
				AddBlocking:  []string{"300", "301"},
			},
		},
		{
			name:  "remove-blocking flag",
			input: "23 --remove-blocking 300",
			output: EditOptions{
				IssueNumbers:   []int{23},
				RemoveBlocking: []string{"300"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, stdin, _, _ := iostreams.Test()
			ios.SetStdoutTTY(true)
			ios.SetStdinTTY(true)
			ios.SetStderrTTY(true)

			if tt.stdin != "" {
				_, _ = stdin.WriteString(tt.stdin)
			}

			f := &cmdutil.Factory{
				IOStreams: ios,
			}

			argv, err := shlex.Split(tt.input)
			assert.NoError(t, err)

			var gotOpts *EditOptions
			cmd := NewCmdEdit(f, func(opts *EditOptions) error {
				gotOpts = opts
				return nil
			})
			cmd.Flags().BoolP("help", "x", false, "")

			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()
			if tt.wantsErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.output.IssueNumbers, gotOpts.IssueNumbers)
			assert.Equal(t, tt.output.Interactive, gotOpts.Interactive)
			assert.Equal(t, tt.output.Editable, gotOpts.Editable)
			assert.Equal(t, tt.output.Parent, gotOpts.Parent)
			assert.Equal(t, tt.output.RemoveParent, gotOpts.RemoveParent)
			assert.Equal(t, tt.output.AddSubIssues, gotOpts.AddSubIssues)
			assert.Equal(t, tt.output.RemoveSubIssues, gotOpts.RemoveSubIssues)
			assert.Equal(t, tt.output.AddBlockedBy, gotOpts.AddBlockedBy)
			assert.Equal(t, tt.output.RemoveBlockedBy, gotOpts.RemoveBlockedBy)
			assert.Equal(t, tt.output.AddBlocking, gotOpts.AddBlocking)
			assert.Equal(t, tt.output.RemoveBlocking, gotOpts.RemoveBlocking)
			if tt.expectedBaseRepo != nil {
				baseRepo, err := gotOpts.BaseRepo()
				require.NoError(t, err)
				require.True(
					t,
					ghrepo.IsSame(tt.expectedBaseRepo, baseRepo),
					"expected base repo %+v, got %+v", tt.expectedBaseRepo, baseRepo,
				)
			}
		})
	}
}

func Test_editRun(t *testing.T) {
	tests := []struct {
		name      string
		input     *EditOptions
		httpStubs func(*testing.T, *httpmock.Registry)
		stdout    string
		stderr    string
		wantErr   bool
	}{
		{
			name: "non-interactive",
			input: &EditOptions{
				Detector:     &fd.EnabledDetectorMock{},
				IssueNumbers: []int{123},
				Interactive:  false,
				Editable: prShared.Editable{
					Title: prShared.EditableString{
						Value:  "new title",
						Edited: true,
					},
					Body: prShared.EditableString{
						Value:  "new body",
						Edited: true,
					},
					Assignees: prShared.EditableAssignees{
						EditableSlice: prShared.EditableSlice{
							Add:    []string{"monalisa", "hubot"},
							Remove: []string{"octocat"},
							Edited: true,
						},
					},
					Labels: prShared.EditableSlice{
						Add:    []string{"feature", "TODO", "bug"},
						Remove: []string{"docs"},
						Edited: true,
					},
					Projects: prShared.EditableProjects{
						EditableSlice: prShared.EditableSlice{
							Add:    []string{"Cleanup", "CleanupV2"},
							Remove: []string{"Roadmap", "RoadmapV2"},
							Edited: true,
						},
					},
					Milestone: prShared.EditableString{
						Value:  "GA",
						Edited: true,
					},
					Metadata: api.RepoMetadataResult{
						Labels: []api.RepoLabel{
							{Name: "docs", ID: "DOCSID"},
						},
					},
				},
				FetchOptions: prShared.FetchOptions,
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIssueGet(t, reg)
				mockIssueProjectItemsGet(t, reg)
				mockRepoMetadata(t, reg)
				mockIssueUpdate(t, reg)
				mockIssueUpdateApiActors(t, reg)
				mockIssueUpdateLabels(t, reg)
				mockProjectV2ItemUpdate(t, reg)
			},
			stdout: "https://github.com/OWNER/REPO/issue/123\n",
		},
		{
			name: "non-interactive multiple issues",
			input: &EditOptions{
				Detector:     &fd.EnabledDetectorMock{},
				IssueNumbers: []int{456, 123},
				Interactive:  false,
				Editable: prShared.Editable{
					Assignees: prShared.EditableAssignees{
						EditableSlice: prShared.EditableSlice{
							Add:    []string{"monalisa", "hubot"},
							Remove: []string{"octocat"},
							Edited: true,
						},
					},
					Labels: prShared.EditableSlice{
						Add:    []string{"feature", "TODO", "bug"},
						Remove: []string{"docs"},
						Edited: true,
					},
					Projects: prShared.EditableProjects{
						EditableSlice: prShared.EditableSlice{
							Add:    []string{"Cleanup", "CleanupV2"},
							Remove: []string{"Roadmap", "RoadmapV2"},
							Edited: true,
						},
					},
					Milestone: prShared.EditableString{
						Value:  "GA",
						Edited: true,
					},
				},
				FetchOptions: prShared.FetchOptions,
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				// Should only be one fetch of metadata.
				mockRepoMetadata(t, reg)
				// All other queries and mutations should be doubled.
				mockIssueNumberGet(t, reg, 123)
				mockIssueNumberGet(t, reg, 456)
				mockIssueProjectItemsGet(t, reg)
				mockIssueProjectItemsGet(t, reg)
				mockIssueUpdate(t, reg)
				mockIssueUpdate(t, reg)
				mockIssueUpdateApiActors(t, reg)
				mockIssueUpdateApiActors(t, reg)
				mockIssueUpdateLabels(t, reg)
				mockIssueUpdateLabels(t, reg)
				mockProjectV2ItemUpdate(t, reg)
				mockProjectV2ItemUpdate(t, reg)
			},
			stdout: heredoc.Doc(`
				https://github.com/OWNER/REPO/issue/123
				https://github.com/OWNER/REPO/issue/456
			`),
		},
		{
			name: "non-interactive multiple issues with fetch failures",
			input: &EditOptions{
				Detector:     &fd.EnabledDetectorMock{},
				IssueNumbers: []int{123, 9999},
				Interactive:  false,
				Editable: prShared.Editable{
					Assignees: prShared.EditableAssignees{
						EditableSlice: prShared.EditableSlice{
							Add:    []string{"monalisa", "hubot"},
							Remove: []string{"octocat"},
							Edited: true,
						},
					},
					Labels: prShared.EditableSlice{
						Add:    []string{"feature", "TODO", "bug"},
						Remove: []string{"docs"},
						Edited: true,
					},
					Projects: prShared.EditableProjects{
						EditableSlice: prShared.EditableSlice{
							Add:    []string{"Cleanup", "CleanupV2"},
							Remove: []string{"Roadmap", "RoadmapV2"},
							Edited: true,
						},
					},
					Milestone: prShared.EditableString{
						Value:  "GA",
						Edited: true,
					},
				},
				FetchOptions: prShared.FetchOptions,
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIssueNumberGet(t, reg, 123)
				reg.Register(
					httpmock.GraphQL(`query IssueByNumber\b`),
					httpmock.StringResponse(`
						{ "errors": [
							{
								"type": "NOT_FOUND",
								"message": "Could not resolve to an Issue with the number of 9999."
							}
						] }`),
				)
			},
			wantErr: true,
		},
		{
			name: "non-interactive multiple issues with update failures",
			input: &EditOptions{
				Detector:     &fd.EnabledDetectorMock{},
				IssueNumbers: []int{123, 456},
				Interactive:  false,
				Editable: prShared.Editable{
					Assignees: prShared.EditableAssignees{
						EditableSlice: prShared.EditableSlice{
							Add:    []string{"monalisa", "hubot"},
							Remove: []string{"octocat"},
							Edited: true,
						},
					},
					Milestone: prShared.EditableString{
						Value:  "GA",
						Edited: true,
					},
				},
				FetchOptions: prShared.FetchOptions,
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				// Should only be one fetch of metadata.
				reg.Register(
					httpmock.GraphQL(`query RepositoryMilestoneList\b`),
					httpmock.StringResponse(`
					{ "data": { "repository": { "milestones": {
						"nodes": [
							{ "title": "GA", "id": "GAID" },
							{ "title": "Big One.oh", "id": "BIGONEID" }
						],
						"pageInfo": { "hasNextPage": false }
					} } } }
					`))
				// All other queries should be doubled.
				mockIssueNumberGet(t, reg, 123)
				mockIssueNumberGet(t, reg, 456)
				// Updating 123 should succeed.
				reg.Register(
					httpmock.GraphQLMutationMatcher(`mutation ReplaceActorsForAssignable\b`, func(m map[string]interface{}) bool {
						return m["assignableId"] == "123"
					}),
					httpmock.GraphQLMutation(`
					{ "data": { "replaceActorsForAssignable": { "__typename": "" } } }`,
						func(inputs map[string]interface{}) {}),
				)
				reg.Register(
					httpmock.GraphQLMutationMatcher(`mutation IssueUpdate\b`, func(m map[string]interface{}) bool {
						return m["id"] == "123"
					}),
					httpmock.GraphQLMutation(`
							{ "data": { "updateIssue": { "__typename": "" } } }`,
						func(inputs map[string]interface{}) {}),
				)
				// Updating 456 should fail.
				reg.Register(
					httpmock.GraphQLMutationMatcher(`mutation ReplaceActorsForAssignable\b`, func(m map[string]interface{}) bool {
						return m["assignableId"] == "456"
					}),
					httpmock.GraphQLMutation(`
							{ "errors": [ { "message": "test error" } ] }`,
						func(inputs map[string]interface{}) {}),
				)
			},
			stdout: heredoc.Doc(`
				https://github.com/OWNER/REPO/issue/123
			`),
			stderr:  `failed to update https://github.com/OWNER/REPO/issue/456:.*test error`,
			wantErr: true,
		},
		{
			name: "interactive",
			input: &EditOptions{
				Detector:     &fd.EnabledDetectorMock{},
				IssueNumbers: []int{123},
				Interactive:  true,
				FieldsToEditSurvey: func(p prShared.EditPrompter, eo *prShared.Editable) error {
					eo.Title.Edited = true
					eo.Body.Edited = true
					eo.Assignees.Edited = true
					eo.Labels.Edited = true
					eo.Projects.Edited = true
					eo.Milestone.Edited = true
					return nil
				},
				EditFieldsSurvey: func(p prShared.EditPrompter, eo *prShared.Editable, _ string) error {
					eo.Title.Value = "new title"
					eo.Body.Value = "new body"
					eo.Assignees.Value = []string{"monalisa", "hubot"}
					eo.Labels.Value = []string{"feature", "TODO", "bug"}
					eo.Labels.Add = []string{"feature", "TODO", "bug"}
					eo.Labels.Remove = []string{"docs"}
					eo.Projects.Value = []string{"Cleanup", "CleanupV2"}
					eo.Milestone.Value = "GA"
					return nil
				},
				FetchOptions:    prShared.FetchOptions,
				DetermineEditor: func() (string, error) { return "vim", nil },
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIssueGet(t, reg)
				mockIssueProjectItemsGet(t, reg)
				mockRepoMetadata(t, reg)
				mockIssueUpdate(t, reg)
				mockIssueUpdateApiActors(t, reg)
				mockIssueUpdateLabels(t, reg)
				mockProjectV2ItemUpdate(t, reg)
			},
			stdout: "https://github.com/OWNER/REPO/issue/123\n",
		},
		{
			name: "interactive prompts with actor assignee display names when actors available",
			input: &EditOptions{
				Detector:     &fd.EnabledDetectorMock{},
				IssueNumbers: []int{123},
				Interactive:  true,
				FieldsToEditSurvey: func(p prShared.EditPrompter, eo *prShared.Editable) error {
					eo.Assignees.Edited = true
					return nil
				},
				EditFieldsSurvey: func(p prShared.EditPrompter, eo *prShared.Editable, _ string) error {
					// Checking that the display name is being used in the prompt.
					require.Equal(t, []string{"hubot"}, eo.Assignees.Default)
					require.Equal(t, []string{"hubot"}, eo.Assignees.DefaultLogins)

					// Adding MonaLisa as issue assignee, should preserve hubot.
					// MultiSelectWithSearch returns Keys (logins), not display names.
					eo.Assignees.Value = []string{"hubot", "MonaLisa"}
					return nil
				},
				FetchOptions:    prShared.FetchOptions,
				DetermineEditor: func() (string, error) { return "vim", nil },
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIsssueNumberGetWithAssignedActors(t, reg, 123)
				mockIssueUpdate(t, reg)
				reg.Register(
					httpmock.GraphQL(`mutation ReplaceActorsForAssignable\b`),
					httpmock.GraphQLMutation(`
					{ "data": { "replaceActorsForAssignable": { "__typename": "" } } }`,
						func(inputs map[string]interface{}) {
							require.Subset(t, inputs["actorLogins"], []interface{}{"hubot", "MonaLisa"})
						}),
				)
			},
			stdout: "https://github.com/OWNER/REPO/issue/123\n",
		},
		{
			name: "interactive prompts with user assignee logins when actors unavailable",
			input: &EditOptions{
				IssueNumbers: []int{123},
				Interactive:  true,
				FieldsToEditSurvey: func(p prShared.EditPrompter, eo *prShared.Editable) error {
					eo.Assignees.Edited = true
					return nil
				},
				EditFieldsSurvey: func(p prShared.EditPrompter, eo *prShared.Editable, _ string) error {
					// Checking that only the login is used in the prompt (no display name)
					require.Equal(t, eo.Assignees.Default, []string{"hubot", "MonaLisa"})

					// Mocking a selection of only MonaLisa in the prompt.
					eo.Assignees.Value = []string{"MonaLisa"}
					return nil
				},
				FetchOptions:    prShared.FetchOptions,
				DetermineEditor: func() (string, error) { return "vim", nil },
				Detector:        &fd.DisabledDetectorMock{},
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query IssueByNumber\b`),
					httpmock.StringResponse(fmt.Sprintf(`
                        { "data": { "repository": { "hasIssuesEnabled": true, "issue": {
							"id": "%[1]d",
							"number": %[1]d,
                            "url": "https://github.com/OWNER/REPO/issue/123",
                            "assignees": {
                                "nodes": [
                                    {
                                        "id": "HUBOTID",
                                        "login": "hubot",
										"name": ""
                                    },
                                    {
                                        "id": "MONAID",
                                        "login": "MonaLisa",
										"name": "Mona Display Name"
                                    }
                                ],
                                "totalCount": 2
                            }
                        } } } }`, 123)),
				)
				reg.Register(
					httpmock.GraphQL(`query RepositoryAssignableUsers\b`),
					httpmock.StringResponse(`
					{ "data": { "repository": { "assignableUsers": {
						"nodes": [
							{ "login": "hubot", "id": "HUBOTID", "name": "" },
							{ "login": "MonaLisa", "id": "MONAID", "name": "Mona Display Name" }
						],
						"pageInfo": { "hasNextPage": false }
					} } } }
					`))
				reg.Register(
					httpmock.GraphQL(`mutation IssueUpdate\b`),
					httpmock.GraphQLMutation(`
								{ "data": { "updateIssue": { "__typename": "" } } }`,
						func(inputs map[string]interface{}) {
							// Checking that we still assigned the expected ID.
							require.Contains(t, inputs["assigneeIds"], "MONAID")
						}),
				)
			},
			stdout: "https://github.com/OWNER/REPO/issue/123\n",
		},
		{
			name: "edit type",
			input: &EditOptions{
				Detector:     &fd.EnabledDetectorMock{},
				IssueNumbers: []int{123},
				Interactive:  false,
				Editable: prShared.Editable{
					IssueType: prShared.EditableString{
						Value:  "Bug",
						Edited: true,
					},
				},
				FetchOptions: prShared.FetchOptions,
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIssueGet(t, reg)
				reg.Register(
					httpmock.GraphQL(`query RepositoryIssueTypes\b`),
					httpmock.StringResponse(`
					{ "data": { "repository": { "issueTypes": { "nodes": [
						{ "id": "BUG_TYPE_ID", "name": "Bug", "description": "", "color": "" },
						{ "id": "FEATURE_TYPE_ID", "name": "Feature", "description": "", "color": "" }
					] } } } }
					`),
				)
				reg.Register(
					httpmock.GraphQL(`mutation UpdateIssueIssueType\b`),
					httpmock.GraphQLMutation(`
					{ "data": { "updateIssueIssueType": { "issue": { "id": "123" } } } }`,
						func(inputs map[string]interface{}) {
							assert.Equal(t, "123", inputs["issueId"])
							assert.Equal(t, "BUG_TYPE_ID", inputs["issueTypeId"])
						}),
				)
			},
			stdout: "https://github.com/OWNER/REPO/issue/123\n",
		},
		{
			name: "remove type",
			input: &EditOptions{
				Detector:        &fd.EnabledDetectorMock{},
				IssueNumbers:    []int{123},
				Interactive:     false,
				RemoveIssueType: true,
				FetchOptions:    prShared.FetchOptions,
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIssueGet(t, reg)
				reg.Register(
					httpmock.GraphQL(`mutation UpdateIssueIssueType\b`),
					httpmock.GraphQLMutation(`
					{ "data": { "updateIssueIssueType": { "issue": { "id": "123" } } } }`,
						func(inputs map[string]interface{}) {
							assert.Equal(t, "123", inputs["issueId"])
							assert.Nil(t, inputs["issueTypeId"])
						}),
				)
			},
			stdout: "https://github.com/OWNER/REPO/issue/123\n",
		},
		{
			name: "interactive edit type prompt",
			input: &EditOptions{
				Detector:     &fd.EnabledDetectorMock{},
				IssueNumbers: []int{123},
				Interactive:  true,
				FieldsToEditSurvey: func(_ prShared.EditPrompter, eo *prShared.Editable) error {
					// Verify the survey is allowed to offer Type as an option for issue edit.
					assert.True(t, eo.IssueType.Selectable)
					eo.IssueType.Edited = true
					return nil
				},
				EditFieldsSurvey: func(_ prShared.EditPrompter, eo *prShared.Editable, _ string) error {
					// FetchOptions populated Options and IssueTypeNameToID from
					// the RepositoryIssueTypes stub below.
					assert.Equal(t, []string{"Bug", "Feature"}, eo.IssueType.Options)
					assert.Equal(t, "FEATURE_TYPE_ID", eo.IssueTypeNameToID["Feature"])
					eo.IssueType.Value = "Feature"
					return nil
				},
				FetchOptions:    prShared.FetchOptions,
				DetermineEditor: func() (string, error) { return "vim", nil },
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIssueGet(t, reg)
				reg.Register(
					httpmock.GraphQL(`query RepositoryIssueTypes\b`),
					httpmock.StringResponse(`
					{ "data": { "repository": { "issueTypes": { "nodes": [
						{ "id": "BUG_TYPE_ID", "name": "Bug", "description": "", "color": "" },
						{ "id": "FEATURE_TYPE_ID", "name": "Feature", "description": "", "color": "" }
					] } } } }
					`),
				)
				reg.Register(
					httpmock.GraphQL(`mutation UpdateIssueIssueType\b`),
					httpmock.GraphQLMutation(`
					{ "data": { "updateIssueIssueType": { "issue": { "id": "123" } } } }`,
						func(inputs map[string]interface{}) {
							assert.Equal(t, "123", inputs["issueId"])
							assert.Equal(t, "FEATURE_TYPE_ID", inputs["issueTypeId"])
						}),
				)
			},
			stdout: "https://github.com/OWNER/REPO/issue/123\n",
		},
		{
			name: "edit set parent",
			input: &EditOptions{
				Detector:     &fd.EnabledDetectorMock{},
				IssueNumbers: []int{123},
				Interactive:  false,
				Parent:       "100",
				FetchOptions: func(_ *api.Client, _ ghrepo.Interface, _ *prShared.Editable, _ gh.ProjectsV1Support) error {
					return nil
				},
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIssueGet(t, reg)
				reg.Register(
					httpmock.GraphQL(`query IssueNodeID\b`),
					httpmock.StringResponse(`
					{ "data": { "repository": { "issue": { "id": "PARENT_100_ID" } } } }
					`),
				)
				reg.Register(
					httpmock.GraphQL(`mutation AddSubIssue\b`),
					httpmock.GraphQLMutation(`
					{ "data": { "addSubIssue": { "issue": { "id": "PARENT_100_ID" } } } }`,
						func(inputs map[string]interface{}) {
							assert.Equal(t, "PARENT_100_ID", inputs["issueId"])
							assert.Equal(t, "123", inputs["subIssueId"])
							assert.Equal(t, true, inputs["replaceParent"])
						}),
				)
			},
			stdout: "https://github.com/OWNER/REPO/issue/123\n",
		},
		{
			name: "edit remove parent",
			input: &EditOptions{
				Detector:     &fd.EnabledDetectorMock{},
				IssueNumbers: []int{123},
				Interactive:  false,
				RemoveParent: true,
				FetchOptions: func(_ *api.Client, _ ghrepo.Interface, _ *prShared.Editable, _ gh.ProjectsV1Support) error {
					return nil
				},
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query IssueByNumber\b`),
					httpmock.StringResponse(`
					{ "data": { "repository": { "hasIssuesEnabled": true, "issue": {
						"id": "123",
						"number": 123,
						"url": "https://github.com/OWNER/REPO/issue/123",
						"parent": {
							"id": "PARENT_100_ID",
							"number": 100,
							"title": "Parent Issue",
							"url": "https://github.com/OWNER/REPO/issues/100",
							"state": "OPEN",
							"repository": { "nameWithOwner": "OWNER/REPO" }
						}
					} } } }
					`),
				)
				reg.Register(
					httpmock.GraphQL(`mutation RemoveSubIssue\b`),
					httpmock.GraphQLMutation(`
					{ "data": { "removeSubIssue": { "issue": { "id": "PARENT_100_ID" } } } }`,
						func(inputs map[string]interface{}) {
							assert.Equal(t, "PARENT_100_ID", inputs["issueId"])
							assert.Equal(t, "123", inputs["subIssueId"])
						}),
				)
			},
			stdout: "https://github.com/OWNER/REPO/issue/123\n",
		},
		{
			name: "edit add sub-issues",
			input: &EditOptions{
				Detector:     &fd.EnabledDetectorMock{},
				IssueNumbers: []int{100},
				Interactive:  false,
				AddSubIssues: []string{"123", "124"},
				FetchOptions: func(_ *api.Client, _ ghrepo.Interface, _ *prShared.Editable, _ gh.ProjectsV1Support) error {
					return nil
				},
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIssueNumberGet(t, reg, 100)
				reg.Register(
					issueNodeIDByNumberMatcher(123),
					httpmock.StringResponse(`{ "data": { "repository": { "issue": { "id": "SUB_123_ID" } } } }`),
				)
				reg.Register(
					issueNodeIDByNumberMatcher(124),
					httpmock.StringResponse(`{ "data": { "repository": { "issue": { "id": "SUB_124_ID" } } } }`),
				)
				reg.Register(
					httpmock.GraphQLMutationMatcher(`mutation AddSubIssue\b`, func(input map[string]interface{}) bool {
						return input["subIssueId"] == "SUB_123_ID"
					}),
					httpmock.GraphQLMutation(`{ "data": { "addSubIssue": { "issue": { "id": "100" } } } }`,
						func(inputs map[string]interface{}) {
							assert.Equal(t, "100", inputs["issueId"])
							assert.Equal(t, true, inputs["replaceParent"])
						}),
				)
				reg.Register(
					httpmock.GraphQLMutationMatcher(`mutation AddSubIssue\b`, func(input map[string]interface{}) bool {
						return input["subIssueId"] == "SUB_124_ID"
					}),
					httpmock.GraphQLMutation(`{ "data": { "addSubIssue": { "issue": { "id": "100" } } } }`,
						func(inputs map[string]interface{}) {
							assert.Equal(t, "100", inputs["issueId"])
							assert.Equal(t, true, inputs["replaceParent"])
						}),
				)
			},
			stdout: "https://github.com/OWNER/REPO/issue/100\n",
		},
		{
			name: "edit remove sub-issue",
			input: &EditOptions{
				Detector:        &fd.EnabledDetectorMock{},
				IssueNumbers:    []int{100},
				Interactive:     false,
				RemoveSubIssues: []string{"123"},
				FetchOptions: func(_ *api.Client, _ ghrepo.Interface, _ *prShared.Editable, _ gh.ProjectsV1Support) error {
					return nil
				},
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIssueNumberGet(t, reg, 100)
				reg.Register(
					httpmock.GraphQL(`query IssueNodeID\b`),
					httpmock.StringResponse(`
					{ "data": { "repository": { "issue": { "id": "SUB_123_ID" } } } }
					`),
				)
				reg.Register(
					httpmock.GraphQL(`mutation RemoveSubIssue\b`),
					httpmock.GraphQLMutation(`
					{ "data": { "removeSubIssue": { "issue": { "id": "100" } } } }`,
						func(inputs map[string]interface{}) {
							assert.Equal(t, "100", inputs["issueId"])
							assert.Equal(t, "SUB_123_ID", inputs["subIssueId"])
						}),
				)
			},
			stdout: "https://github.com/OWNER/REPO/issue/100\n",
		},
		{
			name: "edit add and remove blocked-by",
			input: &EditOptions{
				Detector:        &fd.EnabledDetectorMock{},
				IssueNumbers:    []int{123},
				Interactive:     false,
				AddBlockedBy:    []string{"200"},
				RemoveBlockedBy: []string{"201"},
				FetchOptions: func(_ *api.Client, _ ghrepo.Interface, _ *prShared.Editable, _ gh.ProjectsV1Support) error {
					return nil
				},
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIssueGet(t, reg)
				reg.Register(
					httpmock.GraphQL(`query IssueNodeID\b`),
					httpmock.StringResponse(`
					{ "data": { "repository": { "issue": { "id": "BLOCKING_200_ID" } } } }
					`),
				)
				reg.Register(
					httpmock.GraphQL(`mutation AddBlockedBy\b`),
					httpmock.GraphQLMutation(`
					{ "data": { "addBlockedBy": { "issue": { "id": "123" } } } }`,
						func(inputs map[string]interface{}) {
							assert.Equal(t, "123", inputs["issueId"])
							assert.Equal(t, "BLOCKING_200_ID", inputs["blockingIssueId"])
						}),
				)
				reg.Register(
					httpmock.GraphQL(`query IssueNodeID\b`),
					httpmock.StringResponse(`
					{ "data": { "repository": { "issue": { "id": "BLOCKING_201_ID" } } } }
					`),
				)
				reg.Register(
					httpmock.GraphQL(`mutation RemoveBlockedBy\b`),
					httpmock.GraphQLMutation(`
					{ "data": { "removeBlockedBy": { "issue": { "id": "123" } } } }`,
						func(inputs map[string]interface{}) {
							assert.Equal(t, "123", inputs["issueId"])
							assert.Equal(t, "BLOCKING_201_ID", inputs["blockingIssueId"])
						}),
				)
			},
			stdout: "https://github.com/OWNER/REPO/issue/123\n",
		},
		{
			name: "edit add blocking swaps args",
			input: &EditOptions{
				Detector:     &fd.EnabledDetectorMock{},
				IssueNumbers: []int{123},
				Interactive:  false,
				AddBlocking:  []string{"300"},
				FetchOptions: func(_ *api.Client, _ ghrepo.Interface, _ *prShared.Editable, _ gh.ProjectsV1Support) error {
					return nil
				},
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIssueGet(t, reg)
				reg.Register(
					httpmock.GraphQL(`query IssueNodeID\b`),
					httpmock.StringResponse(`
					{ "data": { "repository": { "issue": { "id": "BLOCKED_300_ID" } } } }
					`),
				)
				reg.Register(
					httpmock.GraphQL(`mutation AddBlockedBy\b`),
					httpmock.GraphQLMutation(`
					{ "data": { "addBlockedBy": { "issue": { "id": "BLOCKED_300_ID" } } } }`,
						func(inputs map[string]interface{}) {
							// --add-blocking swaps: OTHER issue is blocked BY this issue
							assert.Equal(t, "BLOCKED_300_ID", inputs["issueId"])
							assert.Equal(t, "123", inputs["blockingIssueId"])
						}),
				)
			},
			stdout: "https://github.com/OWNER/REPO/issue/123\n",
		},
		{
			name: "edit remove blocking swaps args",
			input: &EditOptions{
				Detector:       &fd.EnabledDetectorMock{},
				IssueNumbers:   []int{123},
				Interactive:    false,
				RemoveBlocking: []string{"300"},
				FetchOptions: func(_ *api.Client, _ ghrepo.Interface, _ *prShared.Editable, _ gh.ProjectsV1Support) error {
					return nil
				},
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIssueGet(t, reg)
				reg.Register(
					httpmock.GraphQL(`query IssueNodeID\b`),
					httpmock.StringResponse(`
					{ "data": { "repository": { "issue": { "id": "BLOCKED_300_ID" } } } }
					`),
				)
				reg.Register(
					httpmock.GraphQL(`mutation RemoveBlockedBy\b`),
					httpmock.GraphQLMutation(`
					{ "data": { "removeBlockedBy": { "issue": { "id": "BLOCKED_300_ID" } } } }`,
						func(inputs map[string]interface{}) {
							// --remove-blocking swaps: OTHER issue is no longer blocked BY this issue
							assert.Equal(t, "BLOCKED_300_ID", inputs["issueId"])
							assert.Equal(t, "123", inputs["blockingIssueId"])
						}),
				)
			},
			stdout: "https://github.com/OWNER/REPO/issue/123\n",
		},
		{
			name: "batch edit type",
			input: &EditOptions{
				Detector:     &fd.EnabledDetectorMock{},
				IssueNumbers: []int{123, 456},
				Interactive:  false,
				Editable: prShared.Editable{
					IssueType: prShared.EditableString{
						Value:  "Bug",
						Edited: true,
					},
				},
				FetchOptions: prShared.FetchOptions,
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryIssueTypes\b`),
					httpmock.StringResponse(`
					{ "data": { "repository": { "issueTypes": { "nodes": [
						{ "id": "BUG_TYPE_ID", "name": "Bug", "description": "", "color": "" }
					] } } } }
					`),
				)
				mockIssueNumberGet(t, reg, 123)
				mockIssueNumberGet(t, reg, 456)
				reg.Register(
					httpmock.GraphQL(`mutation UpdateIssueIssueType\b`),
					httpmock.GraphQLMutation(`
					{ "data": { "updateIssueIssueType": { "issue": { "id": "123" } } } }`,
						func(inputs map[string]interface{}) {}),
				)
				reg.Register(
					httpmock.GraphQL(`mutation UpdateIssueIssueType\b`),
					httpmock.GraphQLMutation(`
					{ "data": { "updateIssueIssueType": { "issue": { "id": "456" } } } }`,
						func(inputs map[string]interface{}) {}),
				)
			},
			stdout: heredoc.Doc(`
				https://github.com/OWNER/REPO/issue/123
				https://github.com/OWNER/REPO/issue/456
			`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(true)
			ios.SetStdinTTY(true)
			ios.SetStderrTTY(true)

			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			tt.httpStubs(t, reg)

			httpClient := func() (*http.Client, error) { return &http.Client{Transport: reg}, nil }
			baseRepo := func() (ghrepo.Interface, error) { return ghrepo.New("OWNER", "REPO"), nil }

			tt.input.IO = ios
			tt.input.HttpClient = httpClient
			tt.input.BaseRepo = baseRepo

			err := editRun(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.stdout, stdout.String())
			// Use regex match since mock errors and service errors will differ.
			assert.Regexp(t, tt.stderr, stderr.String())
		})
	}
}

func mockIssueGet(_ *testing.T, reg *httpmock.Registry) {
	mockIssueNumberGet(nil, reg, 123)
}

func mockIssueNumberGet(_ *testing.T, reg *httpmock.Registry, number int) {
	reg.Register(
		httpmock.GraphQL(`query IssueByNumber\b`),
		httpmock.StringResponse(fmt.Sprintf(`
			{ "data": { "repository": { "hasIssuesEnabled": true, "issue": {
				"id": "%[1]d",
				"number": %[1]d,
				"url": "https://github.com/OWNER/REPO/issue/%[1]d",
				"labels": {
					"nodes": [
						{ "id": "DOCSID", "name": "docs" }
					], "totalCount": 1
				},
				"projectCards": {
					"nodes": [
						{ "project": { "name": "Roadmap" } }
					], "totalCount": 1
				}
			} } } }`, number)),
	)
}

func mockIsssueNumberGetWithAssignedActors(_ *testing.T, reg *httpmock.Registry, number int) {
	reg.Register(
		httpmock.GraphQL(`query IssueByNumber\b`),
		httpmock.StringResponse(fmt.Sprintf(`
			{ "data": { "repository": { "hasIssuesEnabled": true, "issue": {
				"id": "%[1]d",
				"number": %[1]d,
				"url": "https://github.com/OWNER/REPO/issue/%[1]d",
				"assignedActors": {
					"nodes": [
						{
							"id": "HUBOTID",
							"login": "hubot",
							"__typename": "Bot"
						}
					],
					"totalCount": 1
				}
			} } } }`, number)),
	)
}

func mockIssueProjectItemsGet(_ *testing.T, reg *httpmock.Registry) {
	reg.Register(
		httpmock.GraphQL(`query IssueProjectItems\b`),
		httpmock.StringResponse(`
			{ "data": { "repository": { "issue": {
				"projectItems": {
					"nodes": [
						{ "id": "ITEMID", "project": { "title": "RoadmapV2" } }
					]
				}
			} } } }`),
	)
}

func mockRepoMetadata(_ *testing.T, reg *httpmock.Registry) {
	reg.Register(
		httpmock.GraphQL(`query RepositoryLabelList\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": { "labels": {
			"nodes": [
				{ "name": "feature", "id": "FEATUREID" },
				{ "name": "TODO", "id": "TODOID" },
				{ "name": "bug", "id": "BUGID" },
				{ "name": "docs", "id": "DOCSID" }
			],
			"pageInfo": { "hasNextPage": false }
		} } } }
		`))
	reg.Register(
		httpmock.GraphQL(`query RepositoryMilestoneList\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": { "milestones": {
			"nodes": [
				{ "title": "GA", "id": "GAID" },
				{ "title": "Big One.oh", "id": "BIGONEID" }
			],
			"pageInfo": { "hasNextPage": false }
		} } } }
		`))
	reg.Register(
		httpmock.GraphQL(`query RepositoryProjectList\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": { "projects": {
			"nodes": [
				{ "name": "Cleanup", "id": "CLEANUPID" },
				{ "name": "Roadmap", "id": "ROADMAPID" }
			],
			"pageInfo": { "hasNextPage": false }
		} } } }
		`))
	reg.Register(
		httpmock.GraphQL(`query OrganizationProjectList\b`),
		httpmock.StringResponse(`
		{ "data": { "organization": { "projects": {
			"nodes": [
				{ "name": "Triage", "id": "TRIAGEID" }
			],
			"pageInfo": { "hasNextPage": false }
		} } } }
		`))
	reg.Register(
		httpmock.GraphQL(`query RepositoryProjectV2List\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": { "projectsV2": {
			"nodes": [
				{ "title": "CleanupV2", "id": "CLEANUPV2ID" },
				{ "title": "RoadmapV2", "id": "ROADMAPV2ID" }
			],
			"pageInfo": { "hasNextPage": false }
		} } } }
		`))
	reg.Register(
		httpmock.GraphQL(`query OrganizationProjectV2List\b`),
		httpmock.StringResponse(`
		{ "data": { "organization": { "projectsV2": {
			"nodes": [
				{ "title": "TriageV2", "id": "TRIAGEV2ID" }
			],
			"pageInfo": { "hasNextPage": false }
		} } } }
		`))
	reg.Register(
		httpmock.GraphQL(`query UserProjectV2List\b`),
		httpmock.StringResponse(`
		{ "data": { "viewer": { "projectsV2": {
			"nodes": [
				{ "title": "MonalisaV2", "id": "MONALISAV2ID" }
			],
			"pageInfo": { "hasNextPage": false }
		} } } }
		`))
}

func mockIssueUpdate(t *testing.T, reg *httpmock.Registry) {
	reg.Register(
		httpmock.GraphQL(`mutation IssueUpdate\b`),
		httpmock.GraphQLMutation(`
				{ "data": { "updateIssue": { "__typename": "" } } }`,
			func(inputs map[string]interface{}) {}),
	)
}

func mockIssueUpdateApiActors(t *testing.T, reg *httpmock.Registry) {
	reg.Register(
		httpmock.GraphQL(`mutation ReplaceActorsForAssignable\b`),
		httpmock.GraphQLMutation(`
		{ "data": { "replaceActorsForAssignable": { "__typename": "" } } }`,
			func(inputs map[string]interface{}) {}),
	)
}

func mockIssueUpdateLabels(t *testing.T, reg *httpmock.Registry) {
	reg.Register(
		httpmock.GraphQL(`mutation LabelAdd\b`),
		httpmock.GraphQLMutation(`
		{ "data": { "addLabelsToLabelable": { "__typename": "" } } }`,
			func(inputs map[string]interface{}) {}),
	)
	reg.Register(
		httpmock.GraphQL(`mutation LabelRemove\b`),
		httpmock.GraphQLMutation(`
		{ "data": { "removeLabelsFromLabelable": { "__typename": "" } } }`,
			func(inputs map[string]interface{}) {}),
	)
}

func mockProjectV2ItemUpdate(t *testing.T, reg *httpmock.Registry) {
	reg.Register(
		httpmock.GraphQL(`mutation UpdateProjectV2Items\b`),
		httpmock.GraphQLMutation(`
		{ "data": { "add_000": { "item": { "id": "1" } }, "delete_001": { "item": { "id": "2" } } } }`,
			func(inputs map[string]interface{}) {}),
	)
}

// Test_editRun_crossHostRelationshipRefs verifies that every relationship
// flag rejects a cross-host issue URL with the same clear error. Lives as
// its own table rather than additional cases in Test_editRun because each
// case shares identical setup and asserts the same error, varying only in
// which input field carries the cross-host URL.
func Test_editRun_crossHostRelationshipRefs(t *testing.T) {
	const crossHostURL = "https://example.com/OWNER/REPO/issues/9"

	// Each case exercises one relationship-bearing flag with a cross-host
	// URL. ResolveIssueRef should short-circuit before any GraphQL request,
	// and the per-issue failure must surface to stderr.
	tests := []struct {
		name  string
		input *EditOptions
	}{
		{
			name:  "set parent",
			input: &EditOptions{Parent: crossHostURL},
		},
		{
			name:  "add sub-issue",
			input: &EditOptions{AddSubIssues: []string{crossHostURL}},
		},
		{
			name:  "remove sub-issue",
			input: &EditOptions{RemoveSubIssues: []string{crossHostURL}},
		},
		{
			name:  "add blocked-by",
			input: &EditOptions{AddBlockedBy: []string{crossHostURL}},
		},
		{
			name:  "remove blocked-by",
			input: &EditOptions{RemoveBlockedBy: []string{crossHostURL}},
		},
		{
			name:  "add blocking",
			input: &EditOptions{AddBlocking: []string{crossHostURL}},
		},
		{
			name:  "remove blocking",
			input: &EditOptions{RemoveBlocking: []string{crossHostURL}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, stderr := iostreams.Test()
			ios.SetStdoutTTY(true)

			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			mockIssueGet(t, reg)
			// No IssueNodeID stub on purpose: the cross-host guard must
			// short-circuit before any resolution request goes out.

			tt.input.Detector = &fd.EnabledDetectorMock{}
			tt.input.IssueNumbers = []int{123}
			tt.input.Interactive = false
			tt.input.FetchOptions = func(_ *api.Client, _ ghrepo.Interface, _ *prShared.Editable, _ gh.ProjectsV1Support) error {
				return nil
			}
			tt.input.IO = ios
			tt.input.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			}
			tt.input.BaseRepo = func() (ghrepo.Interface, error) {
				return ghrepo.New("OWNER", "REPO"), nil
			}

			err := editRun(tt.input)
			require.Error(t, err)
			assert.Regexp(t, `belongs to a different host \(example\.com\) than the current repository \(github\.com\)`, stderr.String())
		})
	}
}

func TestApiActorsSupported(t *testing.T) {
	t.Run("when actors are assignable, query includes assignedActors", func(t *testing.T) {
		ios, _, _, _ := iostreams.Test()

		reg := &httpmock.Registry{}
		reg.Register(
			httpmock.GraphQL(`assignedActors`),
			// Simulate a GraphQL error to early exit the test.
			httpmock.StatusStringResponse(500, ""),
		)

		_, cmdTeardown := run.Stub()
		defer cmdTeardown(t)

		// Ignore the error because we don't care.
		_ = editRun(&EditOptions{
			IO: ios,
			HttpClient: func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			},
			BaseRepo: func() (ghrepo.Interface, error) {
				return ghrepo.New("OWNER", "REPO"), nil
			},
			Detector:     &fd.EnabledDetectorMock{},
			IssueNumbers: []int{123},
			Editable: prShared.Editable{
				Assignees: prShared.EditableAssignees{
					EditableSlice: prShared.EditableSlice{
						Add:    []string{"monalisa", "octocat"},
						Edited: true,
					},
				},
			},
		})

		reg.Verify(t)
	})

	t.Run("when actors are not assignable, query includes assignees instead", func(t *testing.T) {
		ios, _, _, _ := iostreams.Test()

		reg := &httpmock.Registry{}
		// This test should NOT include assignedActors in the query
		reg.Exclude(t, httpmock.GraphQL(`assignedActors`))
		// It should include the regular assignees field
		reg.Register(
			httpmock.GraphQL(`assignees`),
			// Simulate a GraphQL error to early exit the test.
			httpmock.StatusStringResponse(500, ""),
		)

		_, cmdTeardown := run.Stub()
		defer cmdTeardown(t)

		// Ignore the error because we're not really interested in it.
		_ = editRun(&EditOptions{
			IO: ios,
			HttpClient: func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			},
			BaseRepo: func() (ghrepo.Interface, error) {
				return ghrepo.New("OWNER", "REPO"), nil
			},
			Detector:     &fd.DisabledDetectorMock{},
			IssueNumbers: []int{123},
			Editable: prShared.Editable{
				Assignees: prShared.EditableAssignees{
					EditableSlice: prShared.EditableSlice{
						Add:    []string{"monalisa", "octocat"},
						Edited: true,
					},
				},
			},
		})

		reg.Verify(t)
	})
}

// TODO projectsV1Deprecation
// Remove this test.
func TestProjectsV1Deprecation(t *testing.T) {
	t.Run("when projects v1 is supported, is included in query", func(t *testing.T) {
		ios, _, _, _ := iostreams.Test()

		reg := &httpmock.Registry{}
		reg.Register(
			httpmock.GraphQL(`projectCards`),
			// Simulate a GraphQL error to early exit the test.
			httpmock.StatusStringResponse(500, ""),
		)

		_, cmdTeardown := run.Stub()
		defer cmdTeardown(t)

		// Ignore the error because we have no way to really stub it without
		// fully stubbing a GQL error structure in the request body.
		_ = editRun(&EditOptions{
			IO: ios,
			HttpClient: func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			},
			BaseRepo: func() (ghrepo.Interface, error) {
				return ghrepo.New("OWNER", "REPO"), nil
			},
			Detector: &fd.EnabledDetectorMock{},

			IssueNumbers: []int{123},
			Editable: prShared.Editable{
				Projects: prShared.EditableProjects{
					EditableSlice: prShared.EditableSlice{
						Add:    []string{"Test Project"},
						Edited: true,
					},
				},
			},
		})

		// Verify that our request contained projectCards
		reg.Verify(t)
	})

	t.Run("when projects v1 is not supported, is not included in query", func(t *testing.T) {
		ios, _, _, _ := iostreams.Test()

		reg := &httpmock.Registry{}
		reg.Exclude(t, httpmock.GraphQL(`projectCards`))

		reg.Register(
			httpmock.GraphQL(`.*`),
			// Simulate a GraphQL error to early exit the test.
			httpmock.StatusStringResponse(500, ""),
		)

		_, cmdTeardown := run.Stub()
		defer cmdTeardown(t)

		// Ignore the error because we're not really interested in it.
		_ = editRun(&EditOptions{
			IO: ios,
			HttpClient: func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			},
			BaseRepo: func() (ghrepo.Interface, error) {
				return ghrepo.New("OWNER", "REPO"), nil
			},
			Detector: &fd.DisabledDetectorMock{},

			IssueNumbers: []int{123},
			Editable: prShared.Editable{
				Projects: prShared.EditableProjects{
					EditableSlice: prShared.EditableSlice{
						Add:    []string{"Test Project"},
						Edited: true,
					},
				},
			},
		})

		// Verify that our request contained projectCards
		reg.Verify(t)
	})
}

// issueNodeIDByNumberMatcher matches an IssueNodeID GraphQL query whose
// number variable equals the given value. Used by tests that issue
// multiple IssueNodeID lookups and need stubs to route by issue number
// rather than by registration order.
func issueNodeIDByNumberMatcher(number int) httpmock.Matcher {
	queryMatcher := httpmock.GraphQL(`query IssueNodeID\b`)
	return func(req *http.Request) bool {
		if !queryMatcher(req) {
			return false
		}
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return false
		}
		req.Body = io.NopCloser(bytes.NewReader(body))
		var b struct {
			Variables struct {
				Number int `json:"number"`
			} `json:"variables"`
		}
		if err := json.Unmarshal(body, &b); err != nil {
			return false
		}
		return b.Variables.Number == number
	}
}
