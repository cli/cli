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

func TestNewCmdEdit(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		output  editOptions
		wantErr bool
		errMsg  string
	}{
		{
			name:    "no argument",
			input:   "",
			wantErr: true,
			errMsg:  "cannot update label: name argument required",
		},
		{
			name:    "name argument",
			input:   "test",
			wantErr: true,
			errMsg:  "specify at least one of `--color`, `--description`, or `--name`",
		},
		{
			name:   "description flag",
			input:  "test --description 'some description'",
			output: editOptions{Name: "test", Description: "some description"},
		},
		{
			name:   "color flag",
			input:  "test --color FFFFFF",
			output: editOptions{Name: "test", Color: "FFFFFF"},
		},
		{
			name:   "color flag with pound sign",
			input:  "test --color '#AAAAAA'",
			output: editOptions{Name: "test", Color: "AAAAAA"},
		},
		{
			name:   "name flag",
			input:  "test --name test1",
			output: editOptions{Name: "test", NewName: "test1"},
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
			var gotOpts *editOptions
			cmd := newCmdEdit(f, func(opts *editOptions) error {
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
			assert.Equal(t, tt.output.Color, gotOpts.Color)
			assert.Equal(t, tt.output.Description, gotOpts.Description)
			assert.Equal(t, tt.output.Name, gotOpts.Name)
			assert.Equal(t, tt.output.NewName, gotOpts.NewName)
		})
	}
}

func TestEditRun(t *testing.T) {
	tests := []struct {
		name       string
		tty        bool
		opts       *editOptions
		httpStubs  func(*httpmock.Registry)
		wantStdout string
		wantErrMsg string
	}{
		{
			name: "updates label",
			tty:  true,
			opts: &editOptions{Name: "test", Description: "some description"},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO/labels/test"),
					httpmock.StatusStringResponse(201, "{}"),
				)
			},
			wantStdout: "✓ Label \"test\" updated in OWNER/REPO\n",
		},
		{
			name: "updates label notty",
			tty:  false,
			opts: &editOptions{Name: "test", Description: "some description"},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO/labels/test"),
					httpmock.StatusStringResponse(201, "{}"),
				)
			},
			wantStdout: "",
		},
		{
			name: "updates missing label",
			opts: &editOptions{Name: "invalid", Description: "some description"},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO/labels/invalid"),
					httpmock.WithHeader(
						httpmock.StatusStringResponse(404, `{"message":"Not Found"}`),
						"Content-Type",
						"application/json",
					),
				)
			},
			wantErrMsg: "HTTP 404: Not Found (https://api.github.com/repos/OWNER/REPO/labels/invalid)",
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
			err := editRun(tt.opts)

			if tt.wantErrMsg != "" {
				assert.EqualError(t, err, tt.wantErrMsg)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.wantStdout, stdout.String())
		})
	}
}
