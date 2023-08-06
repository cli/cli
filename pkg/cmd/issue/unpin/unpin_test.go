package unpin

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdPin(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		output  UnpinOptions
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
			input: "6",
			output: UnpinOptions{
				SelectorArg: "6",
			},
		},
		{
			name:  "issue url",
			input: "https://github.com/cli/cli/6",
			output: UnpinOptions{
				SelectorArg: "https://github.com/cli/cli/6",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			ios.SetStdinTTY(true)
			ios.SetStdoutTTY(true)
			f := &cmdutil.Factory{
				IOStreams: ios,
			}
			argv, err := shlex.Split(tt.input)
			assert.NoError(t, err)
			var gotOpts *UnpinOptions
			cmd := NewCmdUnpin(f, func(opts *UnpinOptions) error {
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
		})
	}
}

func TestUnpinRun(t *testing.T) {
	tests := []struct {
		name       string
		tty        bool
		opts       *UnpinOptions
		httpStubs  func(*httpmock.Registry)
		wantStdout string
		wantStderr string
	}{
		{
			name: "unpin issue",
			tty:  true,
			opts: &UnpinOptions{SelectorArg: "20"},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query IssueByNumber\b`),
					httpmock.StringResponse(`
            { "data": { "repository": {
              "issue": { "id": "ISSUE-ID", "number": 20, "title": "Issue Title", "isPinned": true}
            } } }`),
				)
				reg.Register(
					httpmock.GraphQL(`mutation IssueUnpin\b`),
					httpmock.GraphQLMutation(`{"id": "ISSUE-ID"}`,
						func(inputs map[string]interface{}) {
							assert.Equal(t, inputs["issueId"], "ISSUE-ID")
						},
					),
				)
			},
			wantStdout: "",
			wantStderr: "✓ Unpinned issue #20 (Issue Title) from OWNER/REPO\n",
		},
		{
			name: "issue not pinned",
			tty:  true,
			opts: &UnpinOptions{SelectorArg: "20"},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query IssueByNumber\b`),
					httpmock.StringResponse(`
            { "data": { "repository": {
              "issue": { "id": "ISSUE-ID", "number": 20, "title": "Issue Title", "isPinned": false}
            } } }`),
				)
			},
			wantStderr: "! Issue #20 (Issue Title) is not pinned to OWNER/REPO\n",
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

		ios, _, stdout, stderr := iostreams.Test()
		ios.SetStdinTTY(tt.tty)
		ios.SetStdoutTTY(tt.tty)
		tt.opts.IO = ios

		tt.opts.Config = func() (config.Config, error) {
			return config.NewBlankConfig(), nil
		}

		tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
			return ghrepo.New("OWNER", "REPO"), nil
		}

		t.Run(tt.name, func(t *testing.T) {
			defer reg.Verify(t)
			err := unpinRun(tt.opts)
			assert.NoError(t, err)
			assert.Equal(t, tt.wantStdout, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
		})
	}
}
