package edit

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	shared "github.com/cli/cli/v2/pkg/cmd/pr/shared"
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
			name:  "no argument",
			input: "",
			output: EditOptions{
				SelectorArg: "",
				Interactive: true,
			},
			wantsErr: false,
		},
		{
			name:     "two arguments",
			input:    "1 2",
			output:   EditOptions{},
			wantsErr: true,
		},
		{
			name:  "pull request number argument",
			input: "23",
			output: EditOptions{
				SelectorArg: "23",
				Interactive: true,
			},
			wantsErr: false,
		},
		{
			name:  "title flag",
			input: "23 --title test",
			output: EditOptions{
				SelectorArg: "23",
				Editable: shared.Editable{
					Title: shared.EditableString{
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
				SelectorArg: "23",
				Editable: shared.Editable{
					Body: shared.EditableString{
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
				SelectorArg: "23",
				Editable: shared.Editable{
					Body: shared.EditableString{
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
				SelectorArg: "23",
				Editable: shared.Editable{
					Body: shared.EditableString{
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
			name:  "base flag",
			input: "23 --base base-branch-name",
			output: EditOptions{
				SelectorArg: "23",
				Editable: shared.Editable{
					Base: shared.EditableString{
						Value:  "base-branch-name",
						Edited: true,
					},
				},
			},
			wantsErr: false,
		},
		{
			name:  "add-reviewer flag",
			input: "23 --add-reviewer monalisa,owner/core",
			output: EditOptions{
				SelectorArg: "23",
				Editable: shared.Editable{
					Reviewers: shared.EditableSlice{
						Add:    []string{"monalisa", "owner/core"},
						Edited: true,
					},
				},
			},
			wantsErr: false,
		},
		{
			name:  "remove-reviewer flag",
			input: "23 --remove-reviewer monalisa,owner/core",
			output: EditOptions{
				SelectorArg: "23",
				Editable: shared.Editable{
					Reviewers: shared.EditableSlice{
						Remove: []string{"monalisa", "owner/core"},
						Edited: true,
					},
				},
			},
			wantsErr: false,
		},
		{
			name:  "add-assignee flag",
			input: "23 --add-assignee monalisa,hubot",
			output: EditOptions{
				SelectorArg: "23",
				Editable: shared.Editable{
					Assignees: shared.EditableSlice{
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
				SelectorArg: "23",
				Editable: shared.Editable{
					Assignees: shared.EditableSlice{
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
				SelectorArg: "23",
				Editable: shared.Editable{
					Labels: shared.EditableSlice{
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
				SelectorArg: "23",
				Editable: shared.Editable{
					Labels: shared.EditableSlice{
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
				SelectorArg: "23",
				Editable: shared.Editable{
					Projects: shared.EditableProjects{
						EditableSlice: shared.EditableSlice{
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
				SelectorArg: "23",
				Editable: shared.Editable{
					Projects: shared.EditableProjects{
						EditableSlice: shared.EditableSlice{
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
				SelectorArg: "23",
				Editable: shared.Editable{
					Milestone: shared.EditableString{
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
				SelectorArg: "23",
				Editable: shared.Editable{
					Milestone: shared.EditableString{
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
			assert.Equal(t, tt.output.SelectorArg, gotOpts.SelectorArg)
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
	}{
		{
			name: "non-interactive",
			input: &EditOptions{
				SelectorArg: "123",
				Finder: shared.NewMockFinder("123", &api.PullRequest{
					URL: "https://github.com/OWNER/REPO/pull/123",
				}, ghrepo.New("OWNER", "REPO")),
				Interactive: false,
				Editable: shared.Editable{
					Title: shared.EditableString{
						Value:  "new title",
						Edited: true,
					},
					Body: shared.EditableString{
						Value:  "new body",
						Edited: true,
					},
					Base: shared.EditableString{
						Value:  "base-branch-name",
						Edited: true,
					},
					Reviewers: shared.EditableSlice{
						Add:    []string{"OWNER/core", "OWNER/external", "monalisa", "hubot"},
						Remove: []string{"dependabot"},
						Edited: true,
					},
					Assignees: shared.EditableSlice{
						Add:    []string{"monalisa", "hubot"},
						Remove: []string{"octocat"},
						Edited: true,
					},
					Labels: shared.EditableSlice{
						Add:    []string{"feature", "TODO", "bug"},
						Remove: []string{"docs"},
						Edited: true,
					},
					Projects: shared.EditableProjects{
						EditableSlice: shared.EditableSlice{
							Add:    []string{"Cleanup", "CleanupV2"},
							Remove: []string{"Roadmap", "RoadmapV2"},
							Edited: true,
						},
					},
					Milestone: shared.EditableString{
						Value:  "GA",
						Edited: true,
					},
				},
				Fetcher: testFetcher{},
			},
			httpStubs: func(reg *httpmock.Registry) {
				mockRepoMetadata(reg, false)
				mockPullRequestUpdate(reg)
				mockPullRequestUpdateAssignee(reg)
				mockPullRequestUpdateLabels(reg)
				mockPullRequestReviewersUpdate(reg)
				mockProjectV2ItemUpdate(reg)
			},
			stdout: "https://github.com/OWNER/REPO/pull/123\n",
		},
		{
			name: "non-interactive skip reviewers",
			input: &EditOptions{
				SelectorArg: "123",
				Finder: shared.NewMockFinder("123", &api.PullRequest{
					URL: "https://github.com/OWNER/REPO/pull/123",
				}, ghrepo.New("OWNER", "REPO")),
				Interactive: false,
				Editable: shared.Editable{
					Title: shared.EditableString{
						Value:  "new title",
						Edited: true,
					},
					Body: shared.EditableString{
						Value:  "new body",
						Edited: true,
					},
					Base: shared.EditableString{
						Value:  "base-branch-name",
						Edited: true,
					},
					Assignees: shared.EditableSlice{
						Add:    []string{"monalisa", "hubot"},
						Remove: []string{"octocat"},
						Edited: true,
					},
					Labels: shared.EditableSlice{
						Add:    []string{"feature", "TODO", "bug"},
						Remove: []string{"docs"},
						Edited: true,
					},
					Projects: shared.EditableProjects{
						EditableSlice: shared.EditableSlice{
							Add:    []string{"Cleanup", "CleanupV2"},
							Remove: []string{"Roadmap", "RoadmapV2"},
							Edited: true,
						},
					},
					Milestone: shared.EditableString{
						Value:  "GA",
						Edited: true,
					},
				},
				Fetcher: testFetcher{},
			},
			httpStubs: func(reg *httpmock.Registry) {
				mockRepoMetadata(reg, true)
				mockPullRequestUpdate(reg)
				mockPullRequestUpdateAssignee(reg)
				mockPullRequestUpdateLabels(reg)
				mockProjectV2ItemUpdate(reg)
			},
			stdout: "https://github.com/OWNER/REPO/pull/123\n",
		},
		{
			name: "interactive",
			input: &EditOptions{
				SelectorArg: "123",
				Finder: shared.NewMockFinder("123", &api.PullRequest{
					URL: "https://github.com/OWNER/REPO/pull/123",
				}, ghrepo.New("OWNER", "REPO")),
				Interactive:     true,
				Surveyor:        testSurveyor{},
				Fetcher:         testFetcher{},
				EditorRetriever: testEditorRetriever{},
			},
			httpStubs: func(reg *httpmock.Registry) {
				mockRepoMetadata(reg, false)
				mockPullRequestUpdate(reg)
				mockPullRequestReviewersUpdate(reg)
				mockPullRequestUpdateLabels(reg)
				mockProjectV2ItemUpdate(reg)
			},
			stdout: "https://github.com/OWNER/REPO/pull/123\n",
		},
		{
			name: "interactive skip reviewers",
			input: &EditOptions{
				SelectorArg: "123",
				Finder: shared.NewMockFinder("123", &api.PullRequest{
					URL: "https://github.com/OWNER/REPO/pull/123",
				}, ghrepo.New("OWNER", "REPO")),
				Interactive:     true,
				Surveyor:        testSurveyor{skipReviewers: true},
				Fetcher:         testFetcher{},
				EditorRetriever: testEditorRetriever{},
			},
			httpStubs: func(reg *httpmock.Registry) {
				mockRepoMetadata(reg, true)
				mockPullRequestUpdate(reg)
				mockPullRequestUpdateLabels(reg)
				mockProjectV2ItemUpdate(reg)
			},
			stdout: "https://github.com/OWNER/REPO/pull/123\n",
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

			tt.input.IO = ios
			tt.input.HttpClient = httpClient

			err := editRun(tt.input)
			assert.NoError(t, err)
			assert.Equal(t, tt.stdout, stdout.String())
			assert.Equal(t, tt.stderr, stderr.String())
		})
	}
}

func mockRepoMetadata(reg *httpmock.Registry, skipReviewers bool) {
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
	if !skipReviewers {
		reg.Register(
			httpmock.GraphQL(`query OrganizationTeamList\b`),
			httpmock.StringResponse(`
      { "data": { "organization": { "teams": {
        "nodes": [
          { "slug": "external", "id": "EXTERNALID" },
          { "slug": "core", "id": "COREID" }
        ],
        "pageInfo": { "hasNextPage": false }
      } } } }
		`))
		reg.Register(
			httpmock.GraphQL(`query UserCurrent\b`),
			httpmock.StringResponse(`
		  { "data": { "viewer": { "login": "monalisa" } } }
		`))
	}
}

func mockPullRequestUpdate(reg *httpmock.Registry) {
	reg.Register(
		httpmock.GraphQL(`mutation PullRequestUpdate\b`),
		httpmock.StringResponse(`{}`))
}

func mockPullRequestReviewersUpdate(reg *httpmock.Registry) {
	reg.Register(
		httpmock.GraphQL(`mutation PullRequestUpdateRequestReviews\b`),
		httpmock.StringResponse(`{}`))
}

func mockPullRequestUpdateLabels(reg *httpmock.Registry) {
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

func mockPullRequestUpdateAssignee(reg *httpmock.Registry) {
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

type testFetcher struct{}
type testSurveyor struct {
	skipReviewers bool
}
type testEditorRetriever struct{}

func (f testFetcher) EditableOptionsFetch(client *api.Client, repo ghrepo.Interface, opts *shared.Editable) error {
	return shared.FetchOptions(client, repo, opts)
}

func (s testSurveyor) FieldsToEdit(e *shared.Editable) error {
	e.Title.Edited = true
	e.Body.Edited = true
	if !s.skipReviewers {
		e.Reviewers.Edited = true
	}
	e.Assignees.Edited = true
	e.Labels.Edited = true
	e.Projects.Edited = true
	e.Milestone.Edited = true
	return nil
}

func (s testSurveyor) EditFields(e *shared.Editable, _ string) error {
	e.Title.Value = "new title"
	e.Body.Value = "new body"
	if !s.skipReviewers {
		e.Reviewers.Value = []string{"monalisa", "hubot", "OWNER/core", "OWNER/external"}
	}
	e.Assignees.Value = []string{"monalisa", "hubot"}
	e.Labels.Value = []string{"feature", "TODO", "bug"}
	e.Labels.Add = []string{"feature", "TODO", "bug"}
	e.Labels.Remove = []string{"docs"}
	e.Projects.Value = []string{"Cleanup", "CleanupV2"}
	e.Milestone.Value = "GA"
	return nil
}

func (t testEditorRetriever) Retrieve() (string, error) {
	return "vim", nil
}
