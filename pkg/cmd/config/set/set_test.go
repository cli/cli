package set

import (
	"bytes"
	"testing"

	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdConfigSet(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		output   SetOptions
		wantsErr bool
	}{
		{
			name:     "no arguments",
			input:    "",
			output:   SetOptions{},
			wantsErr: true,
		},
		{
			name:     "no value argument",
			input:    "key",
			output:   SetOptions{},
			wantsErr: true,
		},
		{
			name:     "set key value",
			input:    "key value",
			output:   SetOptions{Key: "key", Value: "value"},
			wantsErr: false,
		},
		{
			name:     "set key value with host",
			input:    "key value --host test.com",
			output:   SetOptions{Hostname: "test.com", Key: "key", Value: "value"},
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

			var gotOpts *SetOptions
			cmd := NewCmdConfigSet(f, func(opts *SetOptions) error {
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
			assert.Equal(t, tt.output.Value, gotOpts.Value)
		})
	}
}

func Test_setRun(t *testing.T) {
	tests := []struct {
		name          string
		input         *SetOptions
		expectedValue string
		stdout        string
		stderr        string
		wantsErr      bool
		errMsg        string
	}{
		{
			name: "set key value",
			input: &SetOptions{
				Config: config.ConfigStub{},
				Key:    "editor",
				Value:  "vim",
			},
			expectedValue: "vim",
		},
		{
			name: "set key value scoped by host",
			input: &SetOptions{
				Config:   config.ConfigStub{},
				Hostname: "github.com",
				Key:      "editor",
				Value:    "vim",
			},
			expectedValue: "vim",
		},
		{
			name: "set unknown key",
			input: &SetOptions{
				Config: config.ConfigStub{},
				Key:    "unknownKey",
				Value:  "someValue",
			},
			expectedValue: "someValue",
			stderr:        "! warning: 'unknownKey' is not a known configuration key\n",
		},
		{
			name: "set invalid value",
			input: &SetOptions{
				Config: config.ConfigStub{},
				Key:    "git_protocol",
				Value:  "invalid",
			},
			wantsErr: true,
			errMsg:   "failed to set \"git_protocol\" to \"invalid\": valid values are 'https', 'ssh'",
		},
	}
	for _, tt := range tests {
		io, _, stdout, stderr := iostreams.Test()
		tt.input.IO = io

		t.Run(tt.name, func(t *testing.T) {
			err := setRun(tt.input)
			if tt.wantsErr {
				assert.EqualError(t, err, tt.errMsg)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.stdout, stdout.String())
			assert.Equal(t, tt.stderr, stderr.String())

			val, err := tt.input.Config.Get(tt.input.Hostname, tt.input.Key)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedValue, val)

			val, err = tt.input.Config.Get("", "_written")
			assert.NoError(t, err)
			assert.Equal(t, "true", val)
		})
	}
}
