package root

import (
	"testing"

	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/require"
)

func findHelpTopic(t *testing.T, name string) helpTopic {
	t.Helper()

	for _, ht := range HelpTopics {
		if ht.name == name {
			return ht
		}
	}

	require.FailNowf(t, "", "expected to find a topic with name: %q in topics: %v", name, HelpTopics)
	return helpTopic{}
}

func TestEnvironmentHelp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		topic     string
		args      []string
		flags     []string
		assertion require.ErrorAssertionFunc
	}{
		{
			name:      "valid topic",
			topic:     "environment",
			args:      []string{},
			flags:     []string{},
			assertion: require.NoError,
		},
		{
			name:      "more than zero args",
			topic:     "environment",
			args:      []string{"invalid"},
			flags:     []string{},
			assertion: require.NoError,
		},
		{
			name:      "more than zero flags",
			topic:     "environment",
			args:      []string{},
			flags:     []string{"--invalid"},
			assertion: require.Error,
		},
		{
			name:      "help arg",
			topic:     "environment",
			args:      []string{"help"},
			flags:     []string{},
			assertion: require.NoError,
		},
		{
			name:      "help flag",
			topic:     "environment",
			args:      []string{},
			flags:     []string{"--help"},
			assertion: require.NoError,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ios, _, _, stderr := iostreams.Test()

			cmd := NewCmdHelpTopic(ios, findHelpTopic(t, tt.topic))
			cmd.SetArgs(append(tt.args, tt.flags...))
			cmd.SetOut(stderr)
			cmd.SetErr(stderr)

			_, err := cmd.ExecuteC()
			tt.assertion(t, err)
		})
	}
}
