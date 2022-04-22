package label

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdRemove(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		output  removeOptions
		wantErr bool
		errMsg  string
	}{
		{
			name:    "no argument",
			input:   "",
			wantErr: true,
			errMsg:  "cannot remove label: name argument required",
		},
		{
			name:   "name argument",
			input:  "test",
			output: removeOptions{Name: "test"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: io,
			}
			argv, err := shlex.Split(tt.input)
			assert.NoError(t, err)
			var gotOpts *removeOptions
			cmd := newCmdRemove(f, func(opts *removeOptions) error {
				gotOpts = opts
				return nil
			})
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()
			if tt.wantErr {
				assert.EqualError(t, err, tt.errMsg)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.output.Name, gotOpts.Name)
		})
	}
}

func TestRemoveRun(t *testing.T) {
	tests := []struct {
		name       string
		tty        bool
		opts       *removeOptions
		httpStubs  func(*httpmock.Registry)
		wantStdout string
		wantErr    bool
		errMsg     string
	}{
		{
			name: "removes label",
			tty:  true,
			opts: &removeOptions{Name: "test"},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("DELETE", "repos/OWNER/REPO/labels/test"),
					httpmock.StatusStringResponse(204, "{}"),
				)
			},
			wantStdout: "✓ Label \"test\" removed from OWNER/REPO\n",
		},
		{
			name: "removes label notty",
			tty:  false,
			opts: &removeOptions{Name: "test"},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("DELETE", "repos/OWNER/REPO/labels/test"),
					httpmock.StatusStringResponse(204, "{}"),
				)
			},
			wantStdout: "",
		},
		{
			name: "missing label",
			tty:  false,
			opts: &removeOptions{Name: "missing"},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("DELETE", "repos/OWNER/REPO/labels/missing"),
					httpmock.WithHeader(
						httpmock.StatusStringResponse(422, `
						{
							"message":"Not Found"
						}`),
						"Content-Type",
						"application/json",
					),
				)
			},
			wantErr: true,
			errMsg:  "HTTP 422: Not Found (https://api.github.com/repos/OWNER/REPO/labels/missing)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			if tt.httpStubs != nil {
				tt.httpStubs(reg)
			}
			tt.opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			}
			io, _, stdout, _ := iostreams.Test()
			io.SetStdoutTTY(tt.tty)
			io.SetStdinTTY(tt.tty)
			io.SetStderrTTY(tt.tty)
			tt.opts.IO = io
			tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
				return ghrepo.New("OWNER", "REPO"), nil
			}
			defer reg.Verify(t)
			err := removeRun(tt.opts)

			if tt.wantErr {
				assert.EqualError(t, err, tt.errMsg)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.wantStdout, stdout.String())
		})
	}
}
