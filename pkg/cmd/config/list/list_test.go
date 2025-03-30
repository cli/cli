package list

import (
	"bytes"
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdConfigList(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		output   ListOptions
		wantsErr bool
	}{
		{
			name:     "no arguments",
			input:    "",
			output:   ListOptions{},
			wantsErr: false,
		},
		{
			name:     "list with host",
			input:    "--host HOST.com",
			output:   ListOptions{Hostname: "HOST.com"},
			wantsErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &cmdutil.Factory{
				Config: func() (gh.Config, error) {
					return config.NewBlankConfig(), nil
				},
			}

			argv, err := shlex.Split(tt.input)
			assert.NoError(t, err)

			var gotOpts *ListOptions
			cmd := NewCmdConfigList(f, func(opts *ListOptions) error {
				gotOpts = opts
				return nil
			})
			cmd.Flags().BoolP("help", "x", false, "")

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
			assert.Equal(t, tt.output.Hostname, gotOpts.Hostname)
		})
	}
}

func Test_listRun(t *testing.T) {
	tests := []struct {
		name    string
		input   *ListOptions
		config  gh.Config
		stdout  string
		wantErr bool
	}{
		{
			name: "list",
			config: func() gh.Config {
				cfg := config.NewBlankConfig()
				cfg.Set("HOST", "git_protocol", "ssh")
				cfg.Set("HOST", "editor", "/usr/bin/vim")
				cfg.Set("HOST", "prompt", "disabled")
				cfg.Set("HOST", "prefer_editor_prompt", "enabled")
				cfg.Set("HOST", "pager", "less")
				cfg.Set("HOST", "http_unix_socket", "")
				cfg.Set("HOST", "browser", "brave")
				return cfg
			}(),
			input: &ListOptions{Hostname: "HOST"},
			stdout: `git_protocol=ssh
editor=/usr/bin/vim
prompt=disabled
prefer_editor_prompt=enabled
pager=less
http_unix_socket=
browser=brave
`,
		},
	}

	for _, tt := range tests {
		ios, _, stdout, _ := iostreams.Test()
		tt.input.IO = ios
		tt.input.Config = func() (gh.Config, error) {
			return tt.config, nil
		}

		t.Run(tt.name, func(t *testing.T) {
			err := listRun(tt.input)
			require.NoError(t, err)
			require.Equal(t, tt.stdout, stdout.String())
		})
	}
}
