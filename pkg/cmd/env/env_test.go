package env

import (
	"testing"

	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdEnv(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		flags    []string
		wantsErr bool
	}{
		{
			name:     "no args or flags",
			args:     []string{},
			flags:    []string{},
			wantsErr: false,
		},
		{
			name:     "verbose flag",
			args:     []string{},
			flags:    []string{"--verbose"},
			wantsErr: false,
		},
		{
			name:     "help flag",
			args:     []string{},
			flags:    []string{"--help"},
			wantsErr: false,
		},
		{
			name:     "invalid arg",
			args:     []string{"invalid"},
			flags:    []string{},
			wantsErr: true,
		},
		{
			name:     "invalid flag",
			args:     []string{},
			flags:    []string{"--invalid"},
			wantsErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, _, stdout, stderr := iostreams.Test()

			f := &cmdutil.Factory{
				IOStreams: io,
			}

			cmd := NewCmdEnv(f)
			cmd.SetArgs(append(tt.args, tt.flags...))
			cmd.SetOut(stdout)
			cmd.SetErr(stderr)

			_, err := cmd.ExecuteC()
			if tt.wantsErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}
