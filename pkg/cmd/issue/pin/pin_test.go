package pin

import (
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/issue/argparsetest"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdPin(t *testing.T) {
	// Test shared parsing of issue number / URL.
	argparsetest.TestArgParsing(t, NewCmdPin)
}

func TestPinRun(t *testing.T) {
	tests := []struct {
		name       string
		tty        bool
		opts       *PinOptions
		httpStubs  func(*httpmock.Registry)
		wantStdout string
		wantStderr string
	}{
		{
			name: "pin issue",
			tty:  true,
			opts: &PinOptions{IssueNumber: 20},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query IssueByNumber\b`),
					httpmock.StringResponse(`
            { "data": { "repository": {
              "issue": { "id": "ISSUE-ID", "number": 20, "title": "Issue Title", "isPinned": false}
            } } }`),
				)
				reg.Register(
					httpmock.GraphQL(`mutation IssuePin\b`),
					httpmock.GraphQLMutation(`{"id": "ISSUE-ID"}`,
						func(inputs map[string]interface{}) {
							assert.Equal(t, inputs["issueId"], "ISSUE-ID")
						},
					),
				)
			},
			wantStdout: "",
			wantStderr: "✓ Pinned issue OWNER/REPO#20 (Issue Title)\n",
		},
		{
			name: "issue already pinned",
			tty:  true,
			opts: &PinOptions{IssueNumber: 20},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query IssueByNumber\b`),
					httpmock.StringResponse(`
            { "data": { "repository": {
              "issue": { "id": "ISSUE-ID", "number": 20, "title": "Issue Title", "isPinned": true}
            } } }`),
				)
			},
			wantStderr: "! Issue OWNER/REPO#20 (Issue Title) is already pinned\n",
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

		tt.opts.Config = func() (gh.Config, error) {
			return config.NewBlankConfig(), nil
		}

		tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
			return ghrepo.New("OWNER", "REPO"), nil
		}

		t.Run(tt.name, func(t *testing.T) {
			defer reg.Verify(t)
			err := pinRun(tt.opts)
			assert.NoError(t, err)
			assert.Equal(t, tt.wantStdout, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
		})
	}
}
