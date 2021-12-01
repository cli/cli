package list

import (
	"bytes"
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
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
				Config: func() (config.Config, error) {
					return config.ConfigStub{}, nil
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
		config  config.ConfigStub
		stdout  string
		wantErr bool
	}{
		{
			name: "list",
			config: config.ConfigStub{
				"HOST:git_protocol":     "ssh",
				"HOST:editor":           "/usr/bin/vim",
				"HOST:prompt":           "disabled",
				"HOST:pager":            "less",
				"HOST:http_unix_socket": "",
				"HOST:browser":          "brave",
			},
			input: &ListOptions{Hostname: "HOST"}, // ConfigStub gives empty DefaultHost
			stdout: `git_protocol=ssh
editor=/usr/bin/vim
prompt=disabled
pager=less
http_unix_socket=
browser=brave
`,
		},
	}

	for _, tt := range tests {
		io, _, stdout, _ := iostreams.Test()
		tt.input.IO = io
		tt.input.Config = func() (config.Config, error) {
			return tt.config, nil
		}

		t.Run(tt.name, func(t *testing.T) {
			err := listRun(tt.input)
			assert.NoError(t, err)
			assert.Equal(t, tt.stdout, stdout.String())
			//assert.Equal(t, tt.stderr, stderr.String())
		})
	}
}
