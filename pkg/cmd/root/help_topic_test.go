package root

import (
	"testing"

	"github.com/cli/cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
)

func TestNewStaticHelpTopic(t *testing.T) {
	tests := []struct {
		name     string
		topic    string
		args     []string
		flags    []string
		wantsErr bool
	}{
		{
			name:     "valid topic",
			topic:    "environment",
			args:     []string{},
			flags:    []string{},
			wantsErr: false,
		},
		{
			name:     "invalid topic",
			topic:    "invalid",
			args:     []string{},
			flags:    []string{},
			wantsErr: false,
		},
		{
			name:     "more than zero args",
			topic:    "environment",
			args:     []string{"invalid"},
			flags:    []string{},
			wantsErr: true,
		},
		{
			name:     "more than zero flags",
			topic:    "environment",
			args:     []string{},
			flags:    []string{"--invalid"},
			wantsErr: true,
		},
		{
			name:     "help arg",
			topic:    "environment",
			args:     []string{"help"},
			flags:    []string{},
			wantsErr: true,
		},
		{
			name:     "help flag",
			topic:    "environment",
			args:     []string{},
			flags:    []string{"--help"},
			wantsErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, stdout, stderr := iostreams.Test()

			cmd := NewStaticHelpTopic(tt.topic)
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

func TestNewDyanmicHelpTopic(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		flags    []string
		wantLong string
		wantsErr bool
	}{
		{
			name:     "more than zero args",
			args:     []string{"invalid"},
			flags:    []string{},
			wantLong: "cool",
			wantsErr: true,
		},
		{
			name:     "more than zero flags",
			args:     []string{},
			flags:    []string{"--invalid"},
			wantLong: "cool",
			wantsErr: true,
		},
		{
			name:     "help arg",
			args:     []string{"help"},
			flags:    []string{},
			wantLong: "cool",
			wantsErr: true,
		},
		{
			name:     "help flag",
			args:     []string{},
			flags:    []string{"--help"},
			wantLong: "cool",
			wantsErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, stdout, stderr := iostreams.Test()

			cmd := NewDynamicHelpTopic("topic", "rad", func() string {
				return "cool"
			})
			cmd.SetArgs(append(tt.args, tt.flags...))
			cmd.SetOut(stdout)
			cmd.SetErr(stderr)

			_, err := cmd.ExecuteC()
			if tt.wantsErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			assert.Equal(t, tt.wantLong, cmd.Long)
			assert.Equal(t, "rad", cmd.Short)
		})
	}
}
