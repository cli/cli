package delete

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

func TestNewCmdDelete(t *testing.T) {
	tests := []struct {
		name     string
		cli      string
		wants    deleteOptions
		wantsErr bool
	}{
		{
			name:     "no arguments",
			cli:      "",
			wantsErr: true,
		},
		{
			name:  "id argument",
			cli:   "123",
			wants: deleteOptions{Deployment: "123"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
			}

			argv, err := shlex.Split(tt.cli)
			assert.NoError(t, err)

			var gotOpts deleteOptions
			cmd := NewCmdDelete(f, func(opts *deleteOptions) error {
				gotOpts = *opts
				return nil
			})

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

			assert.Equal(t, tt.wants.Deployment, gotOpts.Deployment)
		})
	}
}

func Test_deleteRun(t *testing.T) {
	tests := []struct {
		name       string
		opts       *deleteOptions
		isTTY      bool
		httpStubs  func(t *testing.T, reg *httpmock.Registry)
		wantStdout string
		wantStderr string
		wantErr    string
	}{
		{
			name: "delete deployment",
			opts: &deleteOptions{
				Deployment: "123456789",
			},
			isTTY: true,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("DELETE", "repos/OWNER/REPO/deployments/123456789"),
					httpmock.StatusStringResponse(204, ""),
				)
			},
			wantStdout: "✓ Deleted deployment 123456789 from OWNER/REPO\n",
		},
		{
			name: "no TTY",
			opts: &deleteOptions{
				Deployment: "123456789",
			},
			isTTY: false,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("DELETE", "repos/OWNER/REPO/deployments/123456789"),
					httpmock.StatusStringResponse(204, ""),
				)
			},
			wantStdout: "",
		},
		{
			name: "missing deployment",
			opts: &deleteOptions{
				Deployment: "123456789",
			},
			isTTY: true,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("DELETE", "repos/OWNER/REPO/deployments/123456789"),
					httpmock.StatusStringResponse(404, ""),
				)
			},
			wantErr:    "could not delete deployment: HTTP 404 (https://api.github.com/repos/OWNER/REPO/deployments/123456789)",
			wantStdout: "",
		},
		{
			name: "validation failed",
			opts: &deleteOptions{
				Deployment: "123456789",
			},
			isTTY: true,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("DELETE", "repos/OWNER/REPO/deployments/123456789"),
					httpmock.StatusStringResponse(422, ""),
				)
			},
			wantErr:    "could not delete deployment: HTTP 422 (https://api.github.com/repos/OWNER/REPO/deployments/123456789)",
			wantStdout: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, _ := iostreams.Test()
			ios.SetStdoutTTY(tt.isTTY)
			ios.SetStdinTTY(tt.isTTY)
			ios.SetStderrTTY(tt.isTTY)

			fakeHTTP := &httpmock.Registry{}
			if tt.httpStubs != nil {
				tt.httpStubs(t, fakeHTTP)
			}
			defer fakeHTTP.Verify(t)

			tt.opts.IO = ios
			tt.opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: fakeHTTP}, nil
			}
			tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
				return ghrepo.FromFullName("OWNER/REPO")
			}

			err := deleteRun(tt.opts)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.wantStdout, stdout.String())
		})
	}
}
