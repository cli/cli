package edit

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/cli/cli/internal/ghrepo"
	prShared "github.com/cli/cli/pkg/cmd/pr/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/httpmock"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdEdit(t *testing.T) {
	tests := []struct {
		name     string
		input    string
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
				SelectorArg: "23",
				Interactive: true,
			},
			wantsErr: false,
		},
		{
			name:  "web flag",
			input: "23 --web",
			output: EditOptions{
				SelectorArg: "23",
				Interactive: true,
				WebMode:     true,
			},
			wantsErr: false,
		},
		{
			name:  "title flag",
			input: "23 --title test",
			output: EditOptions{
				SelectorArg: "23",
				EditableOptions: prShared.EditableOptions{
					Title:       "test",
					TitleEdited: true,
				},
			},
			wantsErr: false,
		},
		{
			name:  "body flag",
			input: "23 --body test",
			output: EditOptions{
				SelectorArg: "23",
				EditableOptions: prShared.EditableOptions{
					Body:       "test",
					BodyEdited: true,
				},
			},
			wantsErr: false,
		},
		{
			name:  "assignee flag",
			input: "23 --assignee monalisa,hubot",
			output: EditOptions{
				SelectorArg: "23",
				EditableOptions: prShared.EditableOptions{
					Assignees:       []string{"monalisa", "hubot"},
					AssigneesEdited: true,
				},
			},
			wantsErr: false,
		},
		{
			name:  "label flag",
			input: "23 --label feature,TODO,bug",
			output: EditOptions{
				SelectorArg: "23",
				EditableOptions: prShared.EditableOptions{
					Labels:       []string{"feature", "TODO", "bug"},
					LabelsEdited: true,
				},
			},
			wantsErr: false,
		},
		{
			name:  "project flag",
			input: "23 --project Cleanup,Roadmap",
			output: EditOptions{
				SelectorArg: "23",
				EditableOptions: prShared.EditableOptions{
					Projects:       []string{"Cleanup", "Roadmap"},
					ProjectsEdited: true,
				},
			},
			wantsErr: false,
		},
		{
			name:  "milestone flag",
			input: "23 --milestone GA",
			output: EditOptions{
				SelectorArg: "23",
				EditableOptions: prShared.EditableOptions{
					Milestone:       "GA",
					MilestoneEdited: true,
				},
			},
			wantsErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, _, _, _ := iostreams.Test()
			io.SetStdoutTTY(true)
			io.SetStdinTTY(true)
			io.SetStderrTTY(true)

			f := &cmdutil.Factory{
				IOStreams: io,
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
			assert.Equal(t, tt.output.WebMode, gotOpts.WebMode)
			assert.Equal(t, tt.output.Interactive, gotOpts.Interactive)
			assert.Equal(t, tt.output.EditableOptions, gotOpts.EditableOptions)
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
	}{
		{
			name: "web mode",
			input: &EditOptions{
				SelectorArg:   "123",
				WebMode:       true,
				OpenInBrowser: func(string) error { return nil },
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIssueGet(t, reg)
			},
			stderr: "Opening github.com/OWNER/REPO/issue/123 in your browser.\n",
		},
		{
			name: "non-interactive",
			input: &EditOptions{
				SelectorArg: "123",
				Interactive: false,
				EditableOptions: prShared.EditableOptions{
					Title:           "new title",
					TitleEdited:     true,
					Body:            "new body",
					BodyEdited:      true,
					Assignees:       []string{"monalisa", "hubot"},
					AssigneesEdited: true,
					Labels:          []string{"feature", "TODO", "bug"},
					LabelsEdited:    true,
					Projects:        []string{"Cleanup", "Roadmap"},
					ProjectsEdited:  true,
					Milestone:       "GA",
					MilestoneEdited: true,
				},
				FetchOptions: prShared.FetchOptions,
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIssueGet(t, reg)
				mockRepoMetadata(t, reg)
				mockIssueUpdate(t, reg)
			},
			stdout: "https://github.com/OWNER/REPO/issue/123\n",
		},
		{
			name: "interactive",
			input: &EditOptions{
				SelectorArg: "123",
				Interactive: true,
				FieldsToEditSurvey: func(eo *prShared.EditableOptions) error {
					eo.TitleEdited = true
					eo.BodyEdited = true
					eo.AssigneesEdited = true
					eo.LabelsEdited = true
					eo.ProjectsEdited = true
					eo.MilestoneEdited = true
					return nil
				},
				EditableSurvey: func(_ string, eo *prShared.EditableOptions) error {
					eo.Title = "new title"
					eo.Body = "new body"
					eo.Assignees = []string{"monalisa", "hubot"}
					eo.Labels = []string{"feature", "TODO", "bug"}
					eo.Projects = []string{"Cleanup", "Roadmap"}
					eo.Milestone = "GA"
					return nil
				},
				FetchOptions:    prShared.FetchOptions,
				DetermineEditor: func() (string, error) { return "vim", nil },
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIssueGet(t, reg)
				mockRepoMetadata(t, reg)
				mockIssueUpdate(t, reg)
			},
			stdout: "https://github.com/OWNER/REPO/issue/123\n",
		},
	}
	for _, tt := range tests {
		io, _, stdout, stderr := iostreams.Test()
		io.SetStdoutTTY(true)
		io.SetStdinTTY(true)
		io.SetStderrTTY(true)

		reg := &httpmock.Registry{}
		defer reg.Verify(t)
		tt.httpStubs(t, reg)

		httpClient := func() (*http.Client, error) { return &http.Client{Transport: reg}, nil }
		baseRepo := func() (ghrepo.Interface, error) { return ghrepo.New("OWNER", "REPO"), nil }

		tt.input.IO = io
		tt.input.HttpClient = httpClient
		tt.input.BaseRepo = baseRepo

		t.Run(tt.name, func(t *testing.T) {
			err := editRun(tt.input)
			assert.NoError(t, err)
			assert.Equal(t, tt.stdout, stdout.String())
			assert.Equal(t, tt.stderr, stderr.String())
		})
	}
}

func mockIssueGet(_ *testing.T, reg *httpmock.Registry) {
	reg.Register(
		httpmock.GraphQL(`query IssueByNumber\b`),
		httpmock.StringResponse(`
			{ "data": { "repository": { "hasIssuesEnabled": true, "issue": {
				"number": 123,
				"url": "https://github.com/OWNER/REPO/issue/123"
			} } } }`),
	)
}

func mockRepoMetadata(_ *testing.T, reg *httpmock.Registry) {
	reg.Register(
		httpmock.GraphQL(`query RepositoryAssignableUsers\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": { "assignableUsers": {
			"nodes": [
				{ "login": "hubot", "id": "HUBOTID" },
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
				{ "name": "bug", "id": "BUGID" }
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
}

func mockIssueUpdate(t *testing.T, reg *httpmock.Registry) {
	reg.Register(
		httpmock.GraphQL(`mutation IssueUpdate\b`),
		httpmock.GraphQLMutation(`
				{ "data": { "updateIssue": { "issue": {
					"id": "123"
				} } } }`,
			func(inputs map[string]interface{}) {}),
	)
}
