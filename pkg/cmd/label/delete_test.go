package label

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdDelete(t *testing.T) {
	tests := []struct {
		name       string
		tty        bool
		input      string
		output     deleteOptions
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:       "no argument",
			input:      "",
			wantErr:    true,
			wantErrMsg: "cannot delete label: name argument required",
		},
		{
			name:   "name argument",
			tty:    true,
			input:  "test",
			output: deleteOptions{Name: "test"},
		},
		{
			name:   "confirm argument",
			input:  "test --yes",
			output: deleteOptions{Name: "test", Confirmed: true},
		},
		{
			name:       "confirm no tty",
			input:      "test",
			wantErr:    true,
			wantErrMsg: "--yes required when not running interactively",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: io,
			}
			io.SetStdinTTY(tt.tty)
			io.SetStdoutTTY(tt.tty)

			argv, err := shlex.Split(tt.input)
			assert.NoError(t, err)
			var gotOpts *deleteOptions
			cmd := newCmdDelete(f, func(opts *deleteOptions) error {
				gotOpts = opts
				return nil
			})
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()
			if tt.wantErr {
				assert.EqualError(t, err, tt.wantErrMsg)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.output.Name, gotOpts.Name)
		})
	}
}

func TestDeleteRun(t *testing.T) {
	tests := []struct {
		name          string
		tty           bool
		opts          *deleteOptions
		httpStubs     func(*httpmock.Registry)
		prompterStubs func(*prompter.PrompterMock)
		wantStdout    string
		wantErr       bool
		errMsg        string
	}{
		{
			name: "deletes label",
			tty:  true,
			opts: &deleteOptions{Name: "test"},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("DELETE", "repos/OWNER/REPO/labels/test"),
					httpmock.StatusStringResponse(204, "{}"),
				)
			},
			prompterStubs: func(pm *prompter.PrompterMock) {
				pm.ConfirmDeletionFunc = func(_ string) error {
					return nil
				}
			},
			wantStdout: "✓ Label \"test\" deleted from OWNER/REPO\n",
		},
		{
			name: "deletes label notty",
			tty:  false,
			opts: &deleteOptions{Name: "test", Confirmed: true},
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
			opts: &deleteOptions{Name: "missing", Confirmed: true},
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

			pm := &prompter.PrompterMock{}
			if tt.prompterStubs != nil {
				tt.prompterStubs(pm)
			}
			tt.opts.Prompter = pm

			io, _, stdout, _ := iostreams.Test()
			io.SetStdoutTTY(tt.tty)
			io.SetStdinTTY(tt.tty)
			io.SetStderrTTY(tt.tty)
			tt.opts.IO = io
			tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
				return ghrepo.New("OWNER", "REPO"), nil
			}
			defer reg.Verify(t)
			err := deleteRun(tt.opts)

			if tt.wantErr {
				assert.EqualError(t, err, tt.errMsg)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.wantStdout, stdout.String())
		})
	}
}
