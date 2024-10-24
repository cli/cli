package edit

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
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
		name     string
		input    string
		stdin    string
		output   EditOptions
		wantsErr bool
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
				SelectorArgs: []string{"23"},
				Interactive:  true,
			},
			wantsErr: false,
		},
		{
			name:  "title flag",
			input: "23 --title test",
			output: EditOptions{
				SelectorArgs: []string{"23"},
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
				SelectorArgs: []string{"23"},
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
				SelectorArgs: []string{"23"},
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
				SelectorArgs: []string{"23"},
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
				SelectorArgs: []string{"23"},
				Editable: prShared.Editable{
					Assignees: prShared.EditableSlice{
						Add:    []string{"monalisa", "hubot"},
						Edited: true,
					},
				},
			},
			wantsErr: false,
		},
		{
			name:  "remove-assignee flag",
			input: "23 --remove-assignee monalisa,hubot",
			output: EditOptions{
				SelectorArgs: []string{"23"},
				Editable: prShared.Editable{
					Assignees: prShared.EditableSlice{
						Remove: []string{"monalisa", "hubot"},
						Edited: true,
					},
				},
			},
			wantsErr: false,
		},
		{
			name:  "add-label flag",
			input: "23 --add-label feature,TODO,bug",
			output: EditOptions{
				SelectorArgs: []string{"23"},
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
				SelectorArgs: []string{"23"},
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
				SelectorArgs: []string{"23"},
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
				SelectorArgs: []string{"23"},
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
				SelectorArgs: []string{"23"},
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
				SelectorArgs: []string{"23"},
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
				SelectorArgs: []string{"23", "34"},
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
			name:     "interactive multiple issues",
			input:    "23 34",
			wantsErr: true,
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
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.output.SelectorArgs, gotOpts.SelectorArgs)
			assert.Equal(t, tt.output.Interactive, gotOpts.Interactive)
			assert.Equal(t, tt.output.Editable, gotOpts.Editable)
		})
	}
}

func Test_editRun(t *testing.T) {
	tests := []struct {
		name      string
		input     *EditOptions
		httpStubs func(*httpmock.Registry)
		stdout    string
		stderr    string
		wantErr   bool
	}{
		{
			name: "non-interactive",
			input: &EditOptions{
				SelectorArgs: []string{"123"},
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
					Assignees: prShared.EditableSlice{
						Add:    []string{"monalisa", "hubot"},
						Remove: []string{"octocat"},
						Edited: true,
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
			httpStubs: func(reg *httpmock.Registry) {
				mockIssueGet(reg)
				mockIssueProjectItemsGet(reg)
				mockRepoMetadata(reg)
				mockIssueUpdate(reg)
				mockIssueUpdateAssignee(reg)
				mockIssueUpdateLabels(reg)
				mockProjectV2ItemUpdate(reg)
			},
			stdout: "https://github.com/OWNER/REPO/issue/123\n",
		},
		{
			name: "non-interactive multiple issues",
			input: &EditOptions{
				SelectorArgs: []string{"456", "123"},
				Interactive:  false,
				Editable: prShared.Editable{
					Assignees: prShared.EditableSlice{
						Add:    []string{"monalisa", "hubot"},
						Remove: []string{"octocat"},
						Edited: true,
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
			httpStubs: func(reg *httpmock.Registry) {
				// Should only be one fetch of metadata.
				mockRepoMetadata(reg)
				// All other queries and mutations should be doubled.
				mockIssueNumberGet(reg, 123)
				mockIssueNumberGet(reg, 456)
				mockIssueProjectItemsGet(reg)
				mockIssueProjectItemsGet(reg)
				mockIssueUpdate(reg)
				mockIssueUpdate(reg)
				mockIssueUpdateAssignee(reg)
				mockIssueUpdateAssignee(reg)
				mockIssueUpdateLabels(reg)
				mockIssueUpdateLabels(reg)
				mockProjectV2ItemUpdate(reg)
				mockProjectV2ItemUpdate(reg)
			},
			stdout: heredoc.Doc(`
				https://github.com/OWNER/REPO/issue/123
				https://github.com/OWNER/REPO/issue/456
			`),
		},
		{
			name: "non-interactive multiple issues with fetch failures",
			input: &EditOptions{
				SelectorArgs: []string{"123", "9999"},
				Interactive:  false,
				Editable: prShared.Editable{
					Assignees: prShared.EditableSlice{
						Add:    []string{"monalisa", "hubot"},
						Remove: []string{"octocat"},
						Edited: true,
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
			httpStubs: func(reg *httpmock.Registry) {
				mockIssueNumberGet(reg, 123)
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
				SelectorArgs: []string{"123", "456"},
				Interactive:  false,
				Editable: prShared.Editable{
					Assignees: prShared.EditableSlice{
						Add:    []string{"monalisa", "hubot"},
						Remove: []string{"octocat"},
						Edited: true,
					},
					Milestone: prShared.EditableString{
						Value:  "GA",
						Edited: true,
					},
				},
				FetchOptions: prShared.FetchOptions,
			},
			httpStubs: func(reg *httpmock.Registry) {
				// Should only be one fetch of metadata.
				reg.Register(
					httpmock.GraphQL(`query RepositoryAssignableUsers\b`),
					httpmock.StringResponse(`
					{ "data": { "repository": { "assignableUsers": {
						"nodes": [
							{ "login": "hubot", "id": "HUBOTID" },
							{ "login": "octocat", "id": "OCTOCATID" },
							{ "login": "MonaLisa", "id": "MONAID" }
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
				// All other queries should be doubled.
				mockIssueNumberGet(reg, 123)
				mockIssueNumberGet(reg, 456)
				// Updating 123 should succeed.
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
					httpmock.GraphQLMutationMatcher(`mutation IssueUpdate\b`, func(m map[string]interface{}) bool {
						return m["id"] == "456"
					}),
					httpmock.GraphQLMutation(`
							{ "errors": [ { "message": "test error" } ] }`,
						func(inputs map[string]interface{}) {}),
				)

				mockIssueUpdateAssignee(reg)
				mockIssueUpdateAssignee(reg)
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
				SelectorArgs: []string{"123"},
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
			httpStubs: func(reg *httpmock.Registry) {
				mockIssueGet(reg)
				mockIssueProjectItemsGet(reg)
				mockRepoMetadata(reg)
				mockIssueUpdate(reg)
				mockIssueUpdateLabels(reg)
				mockProjectV2ItemUpdate(reg)
			},
			stdout: "https://github.com/OWNER/REPO/issue/123\n",
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
			tt.httpStubs(reg)

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

func mockIssueGet(reg *httpmock.Registry) {
	mockIssueNumberGet(reg, 123)
}

func mockIssueNumberGet(reg *httpmock.Registry, number int) {
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

func mockIssueProjectItemsGet(reg *httpmock.Registry) {
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

func mockRepoMetadata(reg *httpmock.Registry) {
	reg.Register(
		httpmock.GraphQL(`query RepositoryAssignableUsers\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": { "assignableUsers": {
			"nodes": [
				{ "login": "hubot", "id": "HUBOTID" },
				{ "login": "octocat", "id": "OCTOCATID" },
				{ "login": "MonaLisa", "id": "MONAID" }
			],
			"pageInfo": { "hasNextPage": false }
		} } } }
		`))
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

func mockIssueUpdate(reg *httpmock.Registry) {
	reg.Register(
		httpmock.GraphQL(`mutation IssueUpdate\b`),
		httpmock.GraphQLMutation(`
				{ "data": { "updateIssue": { "__typename": "" } } }`,
			func(inputs map[string]interface{}) {}),
	)
}

func mockIssueUpdateLabels(reg *httpmock.Registry) {
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

func mockIssueUpdateAssignee(reg *httpmock.Registry) {
	reg.Register(
		httpmock.GraphQL(`mutation AssigneeAdd\b`),
		httpmock.StringResponse(`{}`))
	reg.Register(
		httpmock.GraphQL(`mutation AssigneeRemove\b`),
		httpmock.StringResponse(`{}`))
}

func mockProjectV2ItemUpdate(reg *httpmock.Registry) {
	reg.Register(
		httpmock.GraphQL(`mutation UpdateProjectV2Items\b`),
		httpmock.GraphQLMutation(`
		{ "data": { "add_000": { "item": { "id": "1" } }, "delete_001": { "item": { "id": "2" } } } }`,
			func(inputs map[string]interface{}) {}),
	)
}
