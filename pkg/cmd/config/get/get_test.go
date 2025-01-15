package get

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

func TestNewCmdConfigGet(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		output   GetOptions
		wantsErr bool
	}{
		{
			name:     "no arguments",
			input:    "",
			output:   GetOptions{},
			wantsErr: true,
		},
		{
			name:     "get key",
			input:    "key",
			output:   GetOptions{Key: "key"},
			wantsErr: false,
		},
		{
			name:     "get key with host",
			input:    "key --host test.com",
			output:   GetOptions{Hostname: "test.com", Key: "key"},
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

			var gotOpts *GetOptions
			cmd := NewCmdConfigGet(f, func(opts *GetOptions) error {
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
			assert.Equal(t, tt.output.Key, gotOpts.Key)
		})
	}
}

func Test_getRun(t *testing.T) {
	tests := []struct {
		name   string
		input  *GetOptions
		stdout string
		err    error
	}{
		{
			name: "get key",
			input: &GetOptions{
				Key: "editor",
				Config: func() gh.Config {
					cfg := config.NewBlankConfig()
					cfg.Set("", "editor", "ed")
					return cfg
				}(),
			},
			stdout: "ed\n",
		},
		{
			name: "get key scoped by host",
			input: &GetOptions{
				Hostname: "github.com",
				Key:      "editor",
				Config: func() gh.Config {
					cfg := config.NewBlankConfig()
					cfg.Set("", "editor", "ed")
					cfg.Set("github.com", "editor", "vim")
					return cfg
				}(),
			},
			stdout: "vim\n",
		},
		{
			name: "non-existent key",
			input: &GetOptions{
				Key:    "non-existent",
				Config: config.NewBlankConfig(),
			},
			err: nonExistentKeyError{key: "non-existent"},
		},
	}

	for _, tt := range tests {
		ios, _, stdout, _ := iostreams.Test()
		tt.input.IO = ios

		t.Run(tt.name, func(t *testing.T) {
			err := getRun(tt.input)
			require.Equal(t, err, tt.err)
			require.Equal(t, tt.stdout, stdout.String())
		})
	}
}
