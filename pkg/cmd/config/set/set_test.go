package set

import (
	"bytes"
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
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
			_ = config.StubWriteConfig(t)

			f := &cmdutil.Factory{
				Config: func() (config.Config, error) {
					return config.NewBlankConfig(), nil
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
				Config: config.NewBlankConfig(),
				Key:    "editor",
				Value:  "vim",
			},
			expectedValue: "vim",
		},
		{
			name: "set key value scoped by host",
			input: &SetOptions{
				Config:   config.NewBlankConfig(),
				Hostname: "github.com",
				Key:      "editor",
				Value:    "vim",
			},
			expectedValue: "vim",
		},
		{
			name: "set unknown key",
			input: &SetOptions{
				Config: config.NewBlankConfig(),
				Key:    "unknownKey",
				Value:  "someValue",
			},
			expectedValue: "someValue",
			stderr:        "! warning: 'unknownKey' is not a known configuration key\n",
		},
		{
			name: "set invalid value",
			input: &SetOptions{
				Config: config.NewBlankConfig(),
				Key:    "git_protocol",
				Value:  "invalid",
			},
			wantsErr: true,
			errMsg:   "failed to set \"git_protocol\" to \"invalid\": valid values are 'https', 'ssh'",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = config.StubWriteConfig(t)

			ios, _, stdout, stderr := iostreams.Test()
			tt.input.IO = ios

			err := setRun(tt.input)
			if tt.wantsErr {
				assert.EqualError(t, err, tt.errMsg)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.stdout, stdout.String())
			assert.Equal(t, tt.stderr, stderr.String())

			val, err := tt.input.Config.GetOrDefault(tt.input.Hostname, tt.input.Key)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedValue, val)
		})
	}
}

func Test_ValidateValue(t *testing.T) {
	err := ValidateValue("git_protocol", "sshpps")
	assert.EqualError(t, err, "invalid value")

	err = ValidateValue("git_protocol", "ssh")
	assert.NoError(t, err)

	err = ValidateValue("editor", "vim")
	assert.NoError(t, err)

	err = ValidateValue("got", "123")
	assert.NoError(t, err)

	err = ValidateValue("http_unix_socket", "really_anything/is/allowed/and/net.Dial\\(...\\)/will/ultimately/validate")
	assert.NoError(t, err)
}

func Test_ValidateKey(t *testing.T) {
	err := ValidateKey("invalid")
	assert.EqualError(t, err, "invalid key")

	err = ValidateKey("git_protocol")
	assert.NoError(t, err)

	err = ValidateKey("editor")
	assert.NoError(t, err)

	err = ValidateKey("prompt")
	assert.NoError(t, err)

	err = ValidateKey("pager")
	assert.NoError(t, err)

	err = ValidateKey("http_unix_socket")
	assert.NoError(t, err)

	err = ValidateKey("browser")
	assert.NoError(t, err)
}
