package update

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

func TestNewCmdList(t *testing.T) {
	tests := []struct {
		name     string
		cli      string
		wants    updateOptions
		wantsErr bool
	}{
		{
			name:     "no arguments",
			cli:      "",
			wantsErr: true,
		},
		{
			name: "status flag",
			cli:  "--status in_progress 1234",
			wants: updateOptions{
				Status:       "in_progress",
				Deployment:   "1234",
				AutoInactive: true,
			},
		},
		{
			name: "log-url flag",
			cli:  "--status in_progress --log-url https://example.com 1234",
			wants: updateOptions{
				Status:       "in_progress",
				LogURL:       "https://example.com",
				Deployment:   "1234",
				AutoInactive: true,
			},
		},
		{
			name: "description flag",
			cli:  "--status in_progress --description 'deploying a thing' 1234",
			wants: updateOptions{
				Status:       "in_progress",
				Description:  "deploying a thing",
				Deployment:   "1234",
				AutoInactive: true,
			},
		},
		{
			name: "environment flag",
			cli:  "--status in_progress --environment production 1234",
			wants: updateOptions{
				Status:       "in_progress",
				Environment:  "production",
				Deployment:   "1234",
				AutoInactive: true,
			},
		},
		{
			name: "environment-url flag",
			cli:  "--status in_progress --environment-url https://example.com 1234",
			wants: updateOptions{
				Status:         "in_progress",
				EnvironmentURL: "https://example.com",
				Deployment:     "1234",
				AutoInactive:   true,
			},
		},
		{
			name: "auto-inactive flag",
			cli:  "--status in_progress --auto-inactive=false 1234",
			wants: updateOptions{
				Status:       "in_progress",
				Deployment:   "1234",
				AutoInactive: false,
			},
		},
		{
			name:     "invalid status flag",
			cli:      "--status invalid 1234",
			wantsErr: true,
		},
		{
			name:     "invalid auto-inactive flag",
			cli:      "--status in_progress --auto-inactive=invalid 1234",
			wantsErr: true,
		},
		{
			name:     "invalid description flag",
			cli:      "--status in_progress --description 'this is a really long description that exceeds 140 characters in length this is a really long description that exceeds 140 characters in length' 1234",
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

			var gotOpts updateOptions
			cmd := NewCmdUpdate(f, func(opts *updateOptions) error {
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
			assert.Equal(t, tt.wants.Status, gotOpts.Status)
			assert.Equal(t, tt.wants.LogURL, gotOpts.LogURL)
			assert.Equal(t, tt.wants.Description, gotOpts.Description)
			assert.Equal(t, tt.wants.Environment, gotOpts.Environment)
			assert.Equal(t, tt.wants.EnvironmentURL, gotOpts.EnvironmentURL)
			assert.Equal(t, tt.wants.AutoInactive, gotOpts.AutoInactive)
		})
	}
}

func Test_updateRun(t *testing.T) {
	tests := []struct {
		name       string
		opts       *updateOptions
		isTTY      bool
		httpStubs  func(t *testing.T, reg *httpmock.Registry)
		wantStdout string
		wantStderr string
		wantErr    string
	}{
		{
			name: "update deployment",
			opts: &updateOptions{
				Deployment:   "1234",
				Status:       "in_progress",
				AutoInactive: true,
			},
			isTTY: true,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/deployments/1234/statuses"),
					httpmock.RESTPayload(201, `{}`, func(payload map[string]interface{}) {
						assert.Equal(t, map[string]interface{}{
							"state":         "in_progress",
							"auto_inactive": true,
						}, payload)
					}),
				)
			},
			wantStdout: "✓ Created deployment status in OWNER/REPO\n",
		},
		{
			name: "update deployment with details",
			opts: &updateOptions{
				Deployment:     "1234",
				Status:         "success",
				LogURL:         "https://example.com/logs",
				Description:    "deploying a thing",
				Environment:    "production",
				EnvironmentURL: "https://example.com",
				AutoInactive:   false,
			},
			isTTY: true,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/deployments/1234/statuses"),
					httpmock.RESTPayload(201, `{}`, func(payload map[string]interface{}) {
						assert.Equal(t, map[string]interface{}{
							"state":           "success",
							"log_url":         "https://example.com/logs",
							"description":     "deploying a thing",
							"environment":     "production",
							"environment_url": "https://example.com",
							"auto_inactive":   false,
						}, payload)
					}),
				)
			},
			wantStdout: "✓ Created deployment status in OWNER/REPO\n",
		},
		{
			name: "no TTY",
			opts: &updateOptions{
				Deployment:   "12346",
				Status:       "in_progress",
				AutoInactive: true,
			},
			isTTY: false,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/deployments/12346/statuses"),
					httpmock.RESTPayload(201, `{}`, func(payload map[string]interface{}) {
						assert.Equal(t, map[string]interface{}{
							"state":         "in_progress",
							"auto_inactive": true,
						}, payload)
					}),
				)
			},
		},
		{
			name: "validation failed",
			opts: &updateOptions{
				Deployment:   "1234",
				Status:       "in_progress",
				AutoInactive: true,
			},
			isTTY: true,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/deployments/1234/statuses"),
					httpmock.StatusStringResponse(422, ""),
				)
			},
			wantErr: "failed to create deployment status: HTTP 422 (https://api.github.com/repos/OWNER/REPO/deployments/1234/statuses)",
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

			err := updateRun(tt.opts)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.wantStdout, stdout.String())
		})
	}
}
