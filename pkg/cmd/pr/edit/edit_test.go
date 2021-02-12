package edit

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghrepo"
	shared "github.com/cli/cli/pkg/cmd/pr/shared"
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
			name:  "title flag",
			input: "23 --title test",
			output: EditOptions{
				SelectorArg: "23",
				Editable: shared.Editable{
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
				Editable: shared.Editable{
					Body:       "test",
					BodyEdited: true,
				},
			},
			wantsErr: false,
		},
		{
			name:  "reviewer flag",
			input: "23 --reviewer owner/team,monalisa",
			output: EditOptions{
				SelectorArg: "23",
				Editable: shared.Editable{
					Reviewers:       []string{"owner/team", "monalisa"},
					ReviewersEdited: true,
				},
			},
			wantsErr: false,
		},
		{
			name:  "assignee flag",
			input: "23 --assignee monalisa,hubot",
			output: EditOptions{
				SelectorArg: "23",
				Editable: shared.Editable{
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
				Editable: shared.Editable{
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
				Editable: shared.Editable{
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
				Editable: shared.Editable{
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
			assert.Equal(t, tt.output.Interactive, gotOpts.Interactive)
			assert.Equal(t, tt.output.Editable, gotOpts.Editable)
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
			name: "non-interactive",
			input: &EditOptions{
				SelectorArg: "123",
				Interactive: false,
				Editable: shared.Editable{
					Title:           "new title",
					TitleEdited:     true,
					Body:            "new body",
					BodyEdited:      true,
					Reviewers:       []string{"OWNER/core", "OWNER/external", "monalisa", "hubot"},
					ReviewersEdited: true,
					Assignees:       []string{"monalisa", "hubot"},
					AssigneesEdited: true,
					Labels:          []string{"feature", "TODO", "bug"},
					LabelsEdited:    true,
					Projects:        []string{"Cleanup", "Roadmap"},
					ProjectsEdited:  true,
					Milestone:       "GA",
					MilestoneEdited: true,
				},
				Fetcher: testFetcher{},
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockPullRequestGet(t, reg)
				mockRepoMetadata(t, reg, false)
				mockPullRequestUpdate(t, reg)
				mockPullRequestReviewersUpdate(t, reg)
			},
			stdout: "https://github.com/OWNER/REPO/pull/123\n",
		},
		{
			name: "non-interactive skip reviewers",
			input: &EditOptions{
				SelectorArg: "123",
				Interactive: false,
				Editable: shared.Editable{
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
				Fetcher: testFetcher{},
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockPullRequestGet(t, reg)
				mockRepoMetadata(t, reg, true)
				mockPullRequestUpdate(t, reg)
			},
			stdout: "https://github.com/OWNER/REPO/pull/123\n",
		},
		{
			name: "interactive",
			input: &EditOptions{
				SelectorArg:     "123",
				Interactive:     true,
				Surveyor:        testSurveyor{},
				Fetcher:         testFetcher{},
				EditorRetriever: testEditorRetriever{},
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockPullRequestGet(t, reg)
				mockRepoMetadata(t, reg, false)
				mockPullRequestUpdate(t, reg)
				mockPullRequestReviewersUpdate(t, reg)
			},
			stdout: "https://github.com/OWNER/REPO/pull/123\n",
		},
		{
			name: "interactive skip reviewers",
			input: &EditOptions{
				SelectorArg:     "123",
				Interactive:     true,
				Surveyor:        testSurveyor{skipReviewers: true},
				Fetcher:         testFetcher{},
				EditorRetriever: testEditorRetriever{},
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockPullRequestGet(t, reg)
				mockRepoMetadata(t, reg, true)
				mockPullRequestUpdate(t, reg)
			},
			stdout: "https://github.com/OWNER/REPO/pull/123\n",
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

func mockPullRequestGet(_ *testing.T, reg *httpmock.Registry) {
	reg.Register(
		httpmock.GraphQL(`query PullRequestByNumber\b`),
		httpmock.StringResponse(`
			{ "data": { "repository": { "pullRequest": {
				"id": "456",
				"number": 123,
				"url": "https://github.com/OWNER/REPO/pull/123"
			} } } }`),
	)
}

func mockRepoMetadata(_ *testing.T, reg *httpmock.Registry, skipReviewers bool) {
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
	}
}

func mockPullRequestUpdate(t *testing.T, reg *httpmock.Registry) {
	reg.Register(
		httpmock.GraphQL(`mutation PullRequestUpdate\b`),
		httpmock.GraphQLMutation(`
				{ "data": { "updatePullRequest": { "pullRequest": {
					"id": "456"
				} } } }`,
			func(inputs map[string]interface{}) {}),
	)
}

func mockPullRequestReviewersUpdate(t *testing.T, reg *httpmock.Registry) {
	reg.Register(
		httpmock.GraphQL(`mutation PullRequestUpdateRequestReviews\b`),
		httpmock.GraphQLMutation(`
				{ "data": { "requestReviews": { "pullRequest": {
					"id": "456"
				} } } }`,
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
	e.TitleEdited = true
	e.BodyEdited = true
	if !s.skipReviewers {
		e.ReviewersEdited = true
	}
	e.AssigneesEdited = true
	e.LabelsEdited = true
	e.ProjectsEdited = true
	e.MilestoneEdited = true
	return nil
}

func (s testSurveyor) EditFields(e *shared.Editable, _ string) error {
	e.Title = "new title"
	e.Body = "new body"
	if !s.skipReviewers {
		e.Reviewers = []string{"monalisa", "hubot", "OWNER/core", "OWNER/external"}
	}
	e.Assignees = []string{"monalisa", "hubot"}
	e.Labels = []string{"feature", "TODO", "bug"}
	e.Projects = []string{"Cleanup", "Roadmap"}
	e.Milestone = "GA"
	return nil
}

func (t testEditorRetriever) Retrieve() (string, error) {
	return "vim", nil
}
