package config

import (
	"errors"
	"testing"

	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type configStub map[string]string

func genKey(host, key string) string {
	if host != "" {
		return host + ":" + key
	}
	return key
}

func (c configStub) Get(host, key string) (string, error) {
	val, _, err := c.GetWithSource(host, key)
	return val, err
}

func (c configStub) GetWithSource(host, key string) (string, string, error) {
	if v, found := c[genKey(host, key)]; found {
		return v, "(memory)", nil
	}
	return "", "", errors.New("not found")
}

func (c configStub) Set(host, key, value string) error {
	c[genKey(host, key)] = value
	return nil
}

func (c configStub) Aliases() (*config.AliasConfig, error) {
	return nil, nil
}

func (c configStub) Hosts() ([]string, error) {
	return nil, nil
}

func (c configStub) UnsetHost(hostname string) {
}

func (c configStub) CheckWriteable(host, key string) error {
	return nil
}

func (c configStub) Write() error {
	c["_written"] = "true"
	return nil
}

func TestConfigGet(t *testing.T) {
	tests := []struct {
		name   string
		config configStub
		args   []string
		stdout string
		stderr string
	}{
		{
			name: "get key",
			config: configStub{
				"editor": "ed",
			},
			args:   []string{"editor"},
			stdout: "ed\n",
			stderr: "",
		},
		{
			name: "get key scoped by host",
			config: configStub{
				"editor":            "ed",
				"github.com:editor": "vim",
			},
			args:   []string{"editor", "-h", "github.com"},
			stdout: "vim\n",
			stderr: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, _, stdout, stderr := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: io,
				Config: func() (config.Config, error) {
					return tt.config, nil
				},
			}

			cmd := NewCmdConfigGet(f)
			cmd.Flags().BoolP("help", "x", false, "")
			cmd.SetArgs(tt.args)
			cmd.SetOut(stdout)
			cmd.SetErr(stderr)

			_, err := cmd.ExecuteC()
			require.NoError(t, err)

			assert.Equal(t, tt.stdout, stdout.String())
			assert.Equal(t, tt.stderr, stderr.String())
			assert.Equal(t, "", tt.config["_written"])
		})
	}
}

func TestConfigSet(t *testing.T) {
	tests := []struct {
		name      string
		config    configStub
		args      []string
		expectKey string
		stdout    string
		stderr    string
	}{
		{
			name:      "set key",
			config:    configStub{},
			args:      []string{"editor", "vim"},
			expectKey: "editor",
			stdout:    "",
			stderr:    "",
		},
		{
			name:      "set key scoped by host",
			config:    configStub{},
			args:      []string{"editor", "vim", "-h", "github.com"},
			expectKey: "github.com:editor",
			stdout:    "",
			stderr:    "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, _, stdout, stderr := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: io,
				Config: func() (config.Config, error) {
					return tt.config, nil
				},
			}

			cmd := NewCmdConfigSet(f)
			cmd.Flags().BoolP("help", "x", false, "")
			cmd.SetArgs(tt.args)
			cmd.SetOut(stdout)
			cmd.SetErr(stderr)

			_, err := cmd.ExecuteC()
			require.NoError(t, err)

			assert.Equal(t, tt.stdout, stdout.String())
			assert.Equal(t, tt.stderr, stderr.String())
			assert.Equal(t, "vim", tt.config[tt.expectKey])
			assert.Equal(t, "true", tt.config["_written"])
		})
	}
}
