package close

import (
	"bytes"
	"net/http"
	"testing"

	fd "github.com/cli/cli/v2/internal/featuredetection"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdClose(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		output  CloseOptions
		wantErr bool
		errMsg  string
	}{
		{
			name:    "no argument",
			input:   "",
			wantErr: true,
			errMsg:  "accepts 1 arg(s), received 0",
		},
		{
			name:  "issue number",
			input: "123",
			output: CloseOptions{
				SelectorArg: "123",
			},
		},
		{
			name:  "issue url",
			input: "https://github.com/cli/cli/3",
			output: CloseOptions{
				SelectorArg: "https://github.com/cli/cli/3",
			},
		},
		{
			name:  "comment",
			input: "123 --comment 'closing comment'",
			output: CloseOptions{
				SelectorArg: "123",
				Comment:     "closing comment",
			},
		},
		{
			name:  "reason",
			input: "123 --reason 'not planned'",
			output: CloseOptions{
				SelectorArg: "123",
				Reason:      "not planned",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
			}
			argv, err := shlex.Split(tt.input)
			assert.NoError(t, err)
			var gotOpts *CloseOptions
			cmd := NewCmdClose(f, func(opts *CloseOptions) error {
				gotOpts = opts
				return nil
			})
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Equal(t, tt.errMsg, err.Error())
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.output.SelectorArg, gotOpts.SelectorArg)
			assert.Equal(t, tt.output.Comment, gotOpts.Comment)
			assert.Equal(t, tt.output.Reason, gotOpts.Reason)
		})
	}
}

func TestCloseRun(t *testing.T) {
	tests := []struct {
		name       string
		opts       *CloseOptions
		httpStubs  func(*httpmock.Registry)
		wantStderr string
		wantErr    bool
		errMsg     string
	}{
		{
			name: "close issue by number",
			opts: &CloseOptions{
				SelectorArg: "13",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query IssueByNumber\b`),
					httpmock.StringResponse(`
            { "data": { "repository": {
              "hasIssuesEnabled": true,
              "issue": { "id": "THE-ID", "number": 13, "title": "The title of the issue"}
            } } }`),
				)
				reg.Register(
					httpmock.GraphQL(`mutation IssueClose\b`),
					httpmock.GraphQLMutation(`{"id": "THE-ID"}`,
						func(inputs map[string]interface{}) {
							assert.Equal(t, "THE-ID", inputs["issueId"])
						}),
				)
			},
			wantStderr: "✓ Closed issue OWNER/REPO#13 (The title of the issue)\n",
		},
		{
			name: "close issue with comment",
			opts: &CloseOptions{
				SelectorArg: "13",
				Comment:     "closing comment",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query IssueByNumber\b`),
					httpmock.StringResponse(`
            { "data": { "repository": {
              "hasIssuesEnabled": true,
              "issue": { "id": "THE-ID", "number": 13, "title": "The title of the issue"}
            } } }`),
				)
				reg.Register(
					httpmock.GraphQL(`mutation CommentCreate\b`),
					httpmock.GraphQLMutation(`
            { "data": { "addComment": { "commentEdge": { "node": {
              "url": "https://github.com/OWNER/REPO/issues/123#issuecomment-456"
            } } } } }`,
						func(inputs map[string]interface{}) {
							assert.Equal(t, "THE-ID", inputs["subjectId"])
							assert.Equal(t, "closing comment", inputs["body"])
						}),
				)
				reg.Register(
					httpmock.GraphQL(`mutation IssueClose\b`),
					httpmock.GraphQLMutation(`{"id": "THE-ID"}`,
						func(inputs map[string]interface{}) {
							assert.Equal(t, "THE-ID", inputs["issueId"])
						}),
				)
			},
			wantStderr: "✓ Closed issue OWNER/REPO#13 (The title of the issue)\n",
		},
		{
			name: "close issue with reason",
			opts: &CloseOptions{
				SelectorArg: "13",
				Reason:      "not planned",
				Detector:    &fd.EnabledDetectorMock{},
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query IssueByNumber\b`),
					httpmock.StringResponse(`
            { "data": { "repository": {
              "hasIssuesEnabled": true,
              "issue": { "id": "THE-ID", "number": 13, "title": "The title of the issue"}
            } } }`),
				)
				reg.Register(
					httpmock.GraphQL(`mutation IssueClose\b`),
					httpmock.GraphQLMutation(`{"id": "THE-ID"}`,
						func(inputs map[string]interface{}) {
							assert.Equal(t, 2, len(inputs))
							assert.Equal(t, "THE-ID", inputs["issueId"])
							assert.Equal(t, "NOT_PLANNED", inputs["stateReason"])
						}),
				)
			},
			wantStderr: "✓ Closed issue OWNER/REPO#13 (The title of the issue)\n",
		},
		{
			name: "close issue with reason when reason is not supported",
			opts: &CloseOptions{
				SelectorArg: "13",
				Reason:      "not planned",
				Detector:    &fd.DisabledDetectorMock{},
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query IssueByNumber\b`),
					httpmock.StringResponse(`
            { "data": { "repository": {
              "hasIssuesEnabled": true,
              "issue": { "id": "THE-ID", "number": 13, "title": "The title of the issue"}
            } } }`),
				)
				reg.Register(
					httpmock.GraphQL(`mutation IssueClose\b`),
					httpmock.GraphQLMutation(`{"id": "THE-ID"}`,
						func(inputs map[string]interface{}) {
							assert.Equal(t, 1, len(inputs))
							assert.Equal(t, "THE-ID", inputs["issueId"])
						}),
				)
			},
			wantStderr: "✓ Closed issue OWNER/REPO#13 (The title of the issue)\n",
		},
		{
			name: "issue already closed",
			opts: &CloseOptions{
				SelectorArg: "13",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query IssueByNumber\b`),
					httpmock.StringResponse(`
            { "data": { "repository": {
              "hasIssuesEnabled": true,
              "issue": { "number": 13, "title": "The title of the issue", "state": "CLOSED"}
            } } }`),
				)
			},
			wantStderr: "! Issue OWNER/REPO#13 (The title of the issue) is already closed\n",
		},
		{
			name: "issues disabled",
			opts: &CloseOptions{
				SelectorArg: "13",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query IssueByNumber\b`),
					httpmock.StringResponse(`{
            "data": { "repository": { "hasIssuesEnabled": false, "issue": null } },
            "errors": [ { "type": "NOT_FOUND", "path": [ "repository", "issue" ],
            "message": "Could not resolve to an issue or pull request with the number of 13."
					} ] }`),
				)
			},
			wantErr: true,
			errMsg:  "the 'OWNER/REPO' repository has disabled issues",
		},
	}
	for _, tt := range tests {
		reg := &httpmock.Registry{}
		if tt.httpStubs != nil {
			tt.httpStubs(reg)
		}
		tt.opts.HttpClient = func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		}
		ios, _, _, stderr := iostreams.Test()
		tt.opts.IO = ios
		tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
			return ghrepo.FromFullName("OWNER/REPO")
		}
		t.Run(tt.name, func(t *testing.T) {
			defer reg.Verify(t)

			err := closeRun(tt.opts)
			if tt.wantErr {
				assert.EqualError(t, err, tt.errMsg)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.wantStderr, stderr.String())
		})
	}
}
