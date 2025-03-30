package list

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdList(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
		tty     bool
	}{
		{
			name:    "happy path no arguments",
			args:    []string{},
			wantErr: false,
			tty:     false,
		},
		{
			name:    "too many arguments",
			args:    []string{"foo"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			ios.SetStdinTTY(tt.tty)
			ios.SetStdoutTTY(tt.tty)
			ios.SetStderrTTY(tt.tty)

			f := &cmdutil.Factory{
				IOStreams: ios,
			}
			cmd := NewCmdList(f, func(*ListOptions) error {
				return nil
			})
			cmd.SetArgs(tt.args)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err := cmd.ExecuteC()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestListRun(t *testing.T) {
	tests := []struct {
		name       string
		opts       *ListOptions
		isTTY      bool
		httpStubs  func(t *testing.T, reg *httpmock.Registry)
		wantStdout string
		wantStderr string
		wantErr    bool
		errMsg     string
	}{
		{
			name:  "license list tty",
			isTTY: true,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "licenses"),
					httpmock.StringResponse(`[
						{
							"key": "mit",
							"name": "MIT License",
							"spdx_id": "MIT",
							"url": "https://api.github.com/licenses/mit",
							"node_id": "MDc6TGljZW5zZW1pdA=="
						},
						{
							"key": "lgpl-3.0",
							"name": "GNU Lesser General Public License v3.0",
							"spdx_id": "LGPL-3.0",
							"url": "https://api.github.com/licenses/lgpl-3.0",
							"node_id": "MDc6TGljZW5zZW1pdA=="
						},
						{
							"key": "mpl-2.0",
							"name": "Mozilla Public License 2.0",
							"spdx_id": "MPL-2.0",
							"url": "https://api.github.com/licenses/mpl-2.0",
							"node_id": "MDc6TGljZW5zZW1pdA=="
						},
						{
							"key": "agpl-3.0",
							"name": "GNU Affero General Public License v3.0",
							"spdx_id": "AGPL-3.0",
							"url": "https://api.github.com/licenses/agpl-3.0",
							"node_id": "MDc6TGljZW5zZW1pdA=="
						},
						{
							"key": "unlicense",
							"name": "The Unlicense",
							"spdx_id": "Unlicense",
							"url": "https://api.github.com/licenses/unlicense",
							"node_id": "MDc6TGljZW5zZW1pdA=="
						},
						{
							"key": "apache-2.0",
							"name": "Apache License 2.0",
							"spdx_id": "Apache-2.0",
							"url": "https://api.github.com/licenses/apache-2.0",
							"node_id": "MDc6TGljZW5zZW1pdA=="
						},
						{
							"key": "gpl-3.0",
							"name": "GNU General Public License v3.0",
							"spdx_id": "GPL-3.0",
							"url": "https://api.github.com/licenses/gpl-3.0",
							"node_id": "MDc6TGljZW5zZW1pdA=="
						}
						]`,
					))
			},
			wantStdout: heredoc.Doc(`
				LICENSE KEY  SPDX ID     LICENSE NAME
				mit          MIT         MIT License
				lgpl-3.0     LGPL-3.0    GNU Lesser General Public License v3.0
				mpl-2.0      MPL-2.0     Mozilla Public License 2.0
				agpl-3.0     AGPL-3.0    GNU Affero General Public License v3.0
				unlicense    Unlicense   The Unlicense
				apache-2.0   Apache-2.0  Apache License 2.0
				gpl-3.0      GPL-3.0     GNU General Public License v3.0
			`),
			wantStderr: "",
			opts:       &ListOptions{},
		},
		{
			name:  "license list no license templates tty",
			isTTY: true,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "licenses"),
					httpmock.StringResponse(`[]`),
				)
			},
			wantStdout: "",
			wantStderr: "",
			wantErr:    true,
			errMsg:     "No repository licenses found",
			opts:       &ListOptions{},
		},
	}

	for _, tt := range tests {
		reg := &httpmock.Registry{}
		if tt.httpStubs != nil {
			tt.httpStubs(t, reg)
		}
		tt.opts.Config = func() (gh.Config, error) {
			return config.NewBlankConfig(), nil
		}
		tt.opts.HTTPClient = func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		}
		ios, _, stdout, stderr := iostreams.Test()
		ios.SetStdoutTTY(tt.isTTY)
		ios.SetStdinTTY(tt.isTTY)
		ios.SetStderrTTY(tt.isTTY)
		tt.opts.IO = ios

		t.Run(tt.name, func(t *testing.T) {
			defer reg.Verify(t)
			err := listRun(tt.opts)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Equal(t, tt.errMsg, err.Error())
				return
			}

			assert.Equal(t, tt.wantStdout, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
		})
	}
}
