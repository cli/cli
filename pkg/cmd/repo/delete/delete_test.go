package delete

import (
	"net/http"
	"testing"

	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/prompt"
	"github.com/stretchr/testify/assert"
)

func Test_deleteRun(t *testing.T) {
	tests := []struct {
		name       string
		tty        bool
		opts       *DeleteOptions
		httpStubs  func(*httpmock.Registry)
		askStubs   func(*prompt.AskStubber)
		wantStdout string
		wantErr    bool
		errMsg     string
	}{
		{
			name:       "prompting confirmation tty",
			tty:        true,
			opts:       &DeleteOptions{RepoArg: "OWNER/REPO"},
			wantStdout: "✓ Deleted repository OWNER/REPO\n",
			askStubs: func(q *prompt.AskStubber) {
				// TODO: survey stubber doesn't have WithValidation support
				// so this always passes regardless of prompt input
				q.StubOne("OWNER/REPO")
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("DELETE", "repos/OWNER/REPO"),
					httpmock.StatusStringResponse(204, "{}"))
			},
		},
		{
			name: "confimation no tty",
			opts: &DeleteOptions{
				RepoArg:   "OWNER/REPO",
				Confirmed: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("DELETE", "repos/OWNER/REPO"),
					httpmock.StatusStringResponse(204, "{}"))
			},
		},
		{
			name:    "no confirmation no tty",
			opts:    &DeleteOptions{RepoArg: "OWNER/REPO"},
			wantErr: true,
			errMsg:  "could not prompt: confirmation with prompt or --confirm flag required",
		},
	}
	for _, tt := range tests {
		q, teardown := prompt.InitAskStubber()
		defer teardown()
		if tt.askStubs != nil {
			tt.askStubs(q)
		}

		reg := &httpmock.Registry{}
		if tt.httpStubs != nil {
			tt.httpStubs(reg)
		}
		tt.opts.HttpClient = func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		}

		io, _, stdout, _ := iostreams.Test()
		io.SetStdinTTY(tt.tty)
		io.SetStdoutTTY(tt.tty)
		tt.opts.IO = io

		t.Run(tt.name, func(t *testing.T) {
			defer reg.Verify(t)
			err := deleteRun(tt.opts)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Equal(t, tt.errMsg, err.Error())
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wantStdout, stdout.String())
		})
	}
}
