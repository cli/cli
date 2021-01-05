package comment

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/httpmock"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdComment(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		output   CommentOptions
		wantsErr bool
	}{
		{
			name:     "no arguments",
			input:    "",
			output:   CommentOptions{},
			wantsErr: true,
		},
		{
			name:  "issue number",
			input: "1",
			output: CommentOptions{
				SelectorArg: "1",
				Interactive: true,
				InputType:   0,
				Body:        "",
			},
			wantsErr: false,
		},
		{
			name:  "issue url",
			input: "https://github.com/OWNER/REPO/issues/12",
			output: CommentOptions{
				SelectorArg: "https://github.com/OWNER/REPO/issues/12",
				Interactive: true,
				InputType:   0,
				Body:        "",
			},
			wantsErr: false,
		},
		{
			name:  "body flag",
			input: "1 --body test",
			output: CommentOptions{
				SelectorArg: "1",
				Interactive: false,
				InputType:   inline,
				Body:        "test",
			},
			wantsErr: false,
		},
		{
			name:  "editor flag",
			input: "1 --editor",
			output: CommentOptions{
				SelectorArg: "1",
				Interactive: false,
				InputType:   editor,
				Body:        "",
			},
			wantsErr: false,
		},
		{
			name:  "web flag",
			input: "1 --web",
			output: CommentOptions{
				SelectorArg: "1",
				Interactive: false,
				InputType:   web,
				Body:        "",
			},
			wantsErr: false,
		},
		{
			name:     "editor and web flags",
			input:    "1 --editor --web",
			output:   CommentOptions{},
			wantsErr: true,
		},
		{
			name:     "editor and body flags",
			input:    "1 --editor --body test",
			output:   CommentOptions{},
			wantsErr: true,
		},
		{
			name:     "web and body flags",
			input:    "1 --web --body test",
			output:   CommentOptions{},
			wantsErr: true,
		},
		{
			name:     "editor, web, and body flags",
			input:    "1 --editor --web --body test",
			output:   CommentOptions{},
			wantsErr: true,
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

			var gotOpts *CommentOptions
			cmd := NewCmdComment(f, func(opts *CommentOptions) error {
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
			assert.Equal(t, tt.output.InputType, gotOpts.InputType)
			assert.Equal(t, tt.output.Body, gotOpts.Body)
		})
	}
}

func Test_commentRun(t *testing.T) {
	tests := []struct {
		name      string
		input     *CommentOptions
		httpStubs func(*testing.T, *httpmock.Registry)
		stdout    string
		stderr    string
	}{
		{
			name: "interactive web",
			input: &CommentOptions{
				SelectorArg: "123",
				Interactive: true,
				InputType:   0,
				Body:        "",

				InputTypeSurvey: func() (int, error) { return web, nil },
				OpenInBrowser:   func(string) error { return nil },
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIssueFromNumber(t, reg)
			},
			stderr: "Opening github.com/OWNER/REPO/issues/123 in your browser.\n",
		},
		{
			name: "interactive editor",
			input: &CommentOptions{
				SelectorArg: "123",
				Interactive: true,
				InputType:   0,
				Body:        "",

				EditSurvey:          func() (string, error) { return "comment body", nil },
				InputTypeSurvey:     func() (int, error) { return editor, nil },
				ConfirmSubmitSurvey: func() (bool, error) { return true, nil },
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIssueFromNumber(t, reg)
				mockCommentCreate(t, reg)
			},
			stdout: "? Body <Received>\nhttps://github.com/OWNER/REPO/issues/123#issuecomment-456\n",
		},
		{
			name: "non-interactive web",
			input: &CommentOptions{
				SelectorArg: "123",
				Interactive: false,
				InputType:   web,
				Body:        "",

				OpenInBrowser: func(string) error { return nil },
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIssueFromNumber(t, reg)
			},
			stderr: "Opening github.com/OWNER/REPO/issues/123 in your browser.\n",
		},
		{
			name: "non-interactive editor",
			input: &CommentOptions{
				SelectorArg: "123",
				Interactive: false,
				InputType:   editor,
				Body:        "",

				EditSurvey: func() (string, error) { return "comment body", nil },
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIssueFromNumber(t, reg)
				mockCommentCreate(t, reg)
			},
			stdout: "https://github.com/OWNER/REPO/issues/123#issuecomment-456\n",
		},
		{
			name: "non-interactive inline",
			input: &CommentOptions{
				SelectorArg: "123",
				Interactive: false,
				InputType:   inline,
				Body:        "comment body",
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockIssueFromNumber(t, reg)
				mockCommentCreate(t, reg)
			},
			stdout: "https://github.com/OWNER/REPO/issues/123#issuecomment-456\n",
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

		tt.input.IO = io
		tt.input.HttpClient = func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		}
		tt.input.BaseRepo = func() (ghrepo.Interface, error) {
			return ghrepo.New("OWNER", "REPO"), nil
		}

		t.Run(tt.name, func(t *testing.T) {
			err := commentRun(tt.input)
			assert.NoError(t, err)
			assert.Equal(t, tt.stdout, stdout.String())
			assert.Equal(t, tt.stderr, stderr.String())
		})
	}
}

func mockIssueFromNumber(_ *testing.T, reg *httpmock.Registry) {
	reg.Register(
		httpmock.GraphQL(`query IssueByNumber\b`),
		httpmock.StringResponse(`
			{ "data": { "repository": { "hasIssuesEnabled": true, "issue": {
				"number": 123,
				"url": "https://github.com/OWNER/REPO/issues/123"
			} } } }`),
	)
}

func mockCommentCreate(t *testing.T, reg *httpmock.Registry) {
	reg.Register(
		httpmock.GraphQL(`mutation CommentCreate\b`),
		httpmock.GraphQLMutation(`
		{ "data": { "addComment": { "commentEdge": { "node": {
			"url": "https://github.com/OWNER/REPO/issues/123#issuecomment-456"
		} } } } }`,
			func(inputs map[string]interface{}) {
				assert.Equal(t, "comment body", inputs["body"])
			}),
	)
}
