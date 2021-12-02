package delete

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/prompt"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdDelete(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		tty     bool
		output  DeleteOptions
		wantErr bool
		errMsg  string
	}{
		{
			name:   "confirm flag",
			tty:    true,
			input:  "OWNER/REPO --confirm",
			output: DeleteOptions{RepoArg: "OWNER/REPO", Confirmed: true},
		},
		{
			name:    "no confirmation notty",
			input:   "OWNER/REPO",
			output:  DeleteOptions{RepoArg: "OWNER/REPO"},
			wantErr: true,
			errMsg:  "--confirm required when not running interactively",
		},
		{
			name:   "base repo resolution",
			input:  "",
			tty:    true,
			output: DeleteOptions{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, _, _, _ := iostreams.Test()
			io.SetStdinTTY(tt.tty)
			io.SetStdoutTTY(tt.tty)
			f := &cmdutil.Factory{
				IOStreams: io,
			}
			argv, err := shlex.Split(tt.input)
			assert.NoError(t, err)
			var gotOpts *DeleteOptions
			cmd := NewCmdDelete(f, func(opts *DeleteOptions) error {
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
			assert.Equal(t, tt.output.RepoArg, gotOpts.RepoArg)
		})
	}
}

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
				// TODO: survey stubber doesn't have WithValidator support
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
			name:       "infer base repo",
			tty:        true,
			opts:       &DeleteOptions{},
			wantStdout: "✓ Deleted repository OWNER/REPO\n",
			askStubs: func(q *prompt.AskStubber) {
				q.StubOne("OWNER/REPO")
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("DELETE", "repos/OWNER/REPO"),
					httpmock.StatusStringResponse(204, "{}"))
			},
		},
		{
			name: "confirmation no tty",
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
			name:       "short repo name",
			opts:       &DeleteOptions{RepoArg: "REPO"},
			wantStdout: "✓ Deleted repository OWNER/REPO\n",
			tty:        true,
			askStubs: func(q *prompt.AskStubber) {
				q.StubOne("OWNER/REPO")
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data":{"viewer":{"login":"OWNER"}}}`))
				reg.Register(
					httpmock.REST("DELETE", "repos/OWNER/REPO"),
					httpmock.StatusStringResponse(204, "{}"))
			},
		},
	}
	for _, tt := range tests {
		q, teardown := prompt.InitAskStubber()
		defer teardown()
		if tt.askStubs != nil {
			tt.askStubs(q)
		}

		tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
			return ghrepo.New("OWNER", "REPO"), nil
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
