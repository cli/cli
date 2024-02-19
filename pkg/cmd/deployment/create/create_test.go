package create

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

func TestNewCmdCreate(t *testing.T) {
	tests := []struct {
		name     string
		cli      string
		wants    createOptions
		wantsErr bool
	}{
		{
			name:  "no flags",
			cli:   "",
			wants: createOptions{},
		},
		{
			name: "ref flag",
			cli:  "--ref main",
			wants: createOptions{
				Ref: "main",
			},
		},
		{
			name: "environment flag",
			cli:  "--environment production",
			wants: createOptions{
				Environment: "production",
			},
		},
		{
			name: "ref and environment flags",
			cli:  "--ref main --environment production",
			wants: createOptions{
				Ref:         "main",
				Environment: "production",
			},
		},
		{
			name: "status flag",
			cli:  "--status in_progress",
			wants: createOptions{
				Status: "in_progress",
			},
		},
		{
			name:     "invalid status flag",
			cli:      "--status invalid",
			wantsErr: true,
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

			var gotOpts createOptions
			cmd := NewCmdCreate(f, func(opts *createOptions) error {
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

			assert.Equal(t, tt.wants.Ref, gotOpts.Ref)
			assert.Equal(t, tt.wants.Environment, gotOpts.Environment)
		})
	}
}

func Test_createRun(t *testing.T) {
	tests := []struct {
		name       string
		opts       *createOptions
		isTTY      bool
		httpStubs  func(t *testing.T, reg *httpmock.Registry)
		wantStdout string
		wantStderr string
	}{
		{
			name:  "no flags",
			opts:  &createOptions{},
			isTTY: true,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/deployments"),
					httpmock.RESTPayload(201, `{}`, func(payload map[string]interface{}) {
						assert.Equal(t, map[string]interface{}{
							"ref": "main",
						}, payload)
					}),
				)
			},
			wantStdout: "✓ Created deployment for main in OWNER/REPO\n",
		},
		{
			name: "ref flag",
			opts: &createOptions{
				Ref: "trunk",
			},
			isTTY: true,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/deployments"),
					httpmock.RESTPayload(201, `{}`, func(payload map[string]interface{}) {
						assert.Equal(t, map[string]interface{}{
							"ref": "trunk",
						}, payload)
					}),
				)
			},
			wantStdout: "✓ Created deployment for trunk in OWNER/REPO\n",
		},
		{
			name: "environment flag",
			opts: &createOptions{
				Environment: "production",
			},
			isTTY: true,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/deployments"),
					httpmock.RESTPayload(201, `{}`, func(payload map[string]interface{}) {
						assert.Equal(t, map[string]interface{}{
							"ref":         "main",
							"environment": "production",
						}, payload)
					}),
				)
			},
			wantStdout: "✓ Created deployment for main in OWNER/REPO\n",
		},
		{
			name: "all flags",
			opts: &createOptions{
				Ref:         "trunk",
				Environment: "production",
			},
			isTTY: true,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/deployments"),
					httpmock.RESTPayload(201, `{}`, func(payload map[string]interface{}) {
						assert.Equal(t, map[string]interface{}{
							"ref":         "trunk",
							"environment": "production",
						}, payload)
					}),
				)
			},
			wantStdout: "✓ Created deployment for trunk in OWNER/REPO\n",
		},
		{
			name:  "no TTY",
			opts:  &createOptions{},
			isTTY: false,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/deployments"),
					httpmock.StatusJSONResponse(201, struct {
						ID int `json:"id"`
					}{ID: 1234}),
				)
			},
			wantStdout: "1234\n",
		},
		{
			name: "include status",
			opts: &createOptions{
				Status: "in_progress",
			},
			isTTY: true,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/deployments"),
					httpmock.StatusJSONResponse(201, struct {
						ID int `json:"id"`
					}{ID: 1234}),
				)
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/deployments/1234/statuses"),
					httpmock.StatusStringResponse(201, `{}`),
				)
			},
			wantStdout: "✓ Created deployment for main in OWNER/REPO\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, stderr := iostreams.Test()
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
			tt.opts.Branch = func() (string, error) {
				return "main", nil
			}

			err := createRun(tt.opts)
			assert.NoError(t, err)

			assert.Equal(t, tt.wantStdout, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
		})
	}
}
