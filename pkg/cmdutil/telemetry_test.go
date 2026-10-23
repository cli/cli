package cmdutil_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/cli/cli/v2/internal/gh/ghtelemetry"
	"github.com/cli/cli/v2/internal/telemetry"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordTelemetry(t *testing.T) {
	t.Run("records commands that fail argument validation", func(t *testing.T) {
		t.Parallel()

		// Given a command that requires a positional argument
		recorder := &telemetry.EventRecorderSpy{}
		cmd := &cobra.Command{
			Use:           "list",
			Args:          cobra.ExactArgs(1),
			SilenceErrors: true,
			SilenceUsage:  true,
			RunE:          func(cmd *cobra.Command, args []string) error { return nil },
		}
		cmd.Flags().Bool("web", false, "")
		cmd.SetArgs([]string{"--web"})
		cmdutil.RecordTelemetry(cmd, recorder)

		// When Cobra rejects its arguments
		_, err := cmd.ExecuteC()

		// Then the failure is preserved and the command is still recorded
		require.EqualError(t, err, "accepts 1 arg(s), received 0")
		events := recorder.Events()
		require.Len(t, events, 1)
		assert.Equal(t, "command_invocation", events[0].Type)
		assert.Equal(t, "list", events[0].Dimensions["command"])
		assert.Equal(t, "web", events[0].Dimensions["flags"])
	})

	tests := []struct {
		name  string
		args  []string
		flags string
	}{
		{
			name:  "records sorted flag names without argument or flag values",
			args:  []string{"--web", "--repo", "private/repository", "private argument"},
			flags: "repo,web",
		},
		{
			name:  "accepts positional arguments with nil Args and records empty flags",
			args:  []string{"private argument"},
			flags: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Given a command without argument validation
			recorder := &telemetry.EventRecorderSpy{}
			cmd := &cobra.Command{
				Use:  "list",
				RunE: func(cmd *cobra.Command, args []string) error { return nil },
			}
			cmd.Flags().Bool("web", false, "")
			cmd.Flags().String("repo", "", "")
			cmd.Flags().Bool("unused", false, "")
			parent := &cobra.Command{Use: "pr"}
			root := &cobra.Command{Use: "gh"}
			root.AddCommand(parent)
			parent.AddCommand(cmd)
			root.SetArgs(append([]string{"pr", "list"}, tt.args...))
			cmdutil.RecordTelemetry(cmd, recorder)

			// When Cobra executes the command
			_, err := root.ExecuteC()

			// Then only the command path and explicitly supplied flag names are recorded
			require.NoError(t, err)
			events := recorder.Events()
			require.Len(t, events, 1)
			assert.Equal(t, "command_invocation", events[0].Type)
			assert.Equal(t, ghtelemetry.Dimensions{
				"command": "gh pr list",
				"flags":   tt.flags,
			}, events[0].Dimensions)
		})
	}

	t.Run("is a no-op when original RunE is nil", func(t *testing.T) {
		t.Parallel()

		// Given a command using Run instead of RunE
		recorder := &telemetry.EventRecorderSpy{}
		output := &bytes.Buffer{}
		cmd := &cobra.Command{
			Use: "test",
			Run: func(cmd *cobra.Command, args []string) {
				fmt.Fprintln(cmd.OutOrStdout(), "command output")
			},
		}
		cmd.SetOut(output)
		cmd.SetArgs([]string{})
		cmdutil.RecordTelemetry(cmd, recorder)

		// When Cobra executes the command
		_, err := cmd.ExecuteC()

		// Then its original behavior is retained without telemetry
		require.NoError(t, err)
		assert.Equal(t, "command output\n", output.String())
		assert.Empty(t, recorder.Events())
	})

	t.Run("records flags parsed during RunE even when execution fails", func(t *testing.T) {
		t.Parallel()

		// Given a command that manually parses flags before execution fails
		recorder := &telemetry.EventRecorderSpy{}
		expectedErr := fmt.Errorf("something went wrong")
		cmd := &cobra.Command{
			Use:                "copilot",
			DisableFlagParsing: true,
			SilenceErrors:      true,
			SilenceUsage:       true,
			RunE: func(cmd *cobra.Command, args []string) error {
				cmd.DisableFlagParsing = false
				if err := cmd.ParseFlags(args); err != nil {
					return err
				}
				return expectedErr
			},
		}
		cmd.Flags().Bool("remove", false, "")
		cmd.SetArgs([]string{"--remove"})
		cmdutil.RecordTelemetry(cmd, recorder)

		// When Cobra executes the command
		_, err := cmd.ExecuteC()

		// Then the error is preserved and the late-parsed flag is recorded
		require.ErrorIs(t, err, expectedErr)
		events := recorder.Events()
		require.Len(t, events, 1)
		assert.Equal(t, "command_invocation", events[0].Type)
		assert.Equal(t, "copilot", events[0].Dimensions["command"])
		assert.Equal(t, "remove", events[0].Dimensions["flags"])
	})

	t.Run("records commands rejected by a parent persistent pre-run", func(t *testing.T) {
		t.Parallel()

		// Given a command whose parent rejects execution before RunE
		recorder := &telemetry.EventRecorderSpy{}
		expectedErr := fmt.Errorf("authentication required")
		root := &cobra.Command{
			Use:               "gh",
			SilenceErrors:     true,
			SilenceUsage:      true,
			PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return expectedErr },
		}
		cmd := &cobra.Command{
			Use:  "list",
			RunE: func(cmd *cobra.Command, args []string) error { return nil },
		}
		cmd.Flags().Bool("web", false, "")
		root.AddCommand(cmd)
		root.SetArgs([]string{"list", "--web"})
		cmdutil.RecordTelemetry(cmd, recorder)

		// When Cobra rejects the command
		_, err := root.ExecuteC()

		// Then the parent error is preserved and the attempted command is recorded
		require.ErrorIs(t, err, expectedErr)
		events := recorder.Events()
		require.Len(t, events, 1)
		assert.Equal(t, "command_invocation", events[0].Type)
		assert.Equal(t, "gh list", events[0].Dimensions["command"])
		assert.Equal(t, "web", events[0].Dimensions["flags"])
	})

	t.Run("records commands rejected by their pre-run", func(t *testing.T) {
		t.Parallel()

		// Given a command whose pre-run validation fails
		recorder := &telemetry.EventRecorderSpy{}
		expectedErr := fmt.Errorf("incompatible flags")
		cmd := &cobra.Command{
			Use:           "list",
			SilenceErrors: true,
			SilenceUsage:  true,
			PreRunE:       func(cmd *cobra.Command, args []string) error { return expectedErr },
			RunE:          func(cmd *cobra.Command, args []string) error { return nil },
		}
		cmd.SetArgs([]string{})
		cmdutil.RecordTelemetry(cmd, recorder)

		// When Cobra rejects the command
		_, err := cmd.ExecuteC()

		// Then the validation error is preserved and the attempted command is recorded
		require.ErrorIs(t, err, expectedErr)
		events := recorder.Events()
		require.Len(t, events, 1)
		assert.Equal(t, "command_invocation", events[0].Type)
		assert.Equal(t, "list", events[0].Dimensions["command"])
	})

	t.Run("skips commands with telemetry disabled", func(t *testing.T) {
		t.Parallel()

		// Given a command with telemetry disabled
		recorder := &telemetry.EventRecorderSpy{}
		cmd := &cobra.Command{
			Use:  "internal",
			RunE: func(cmd *cobra.Command, args []string) error { return nil },
		}
		cmd.SetArgs([]string{})
		cmdutil.DisableTelemetry(cmd)
		cmdutil.RecordTelemetry(cmd, recorder)

		// When Cobra executes the command
		_, err := cmd.ExecuteC()

		// Then the command succeeds without recording telemetry
		require.NoError(t, err)
		assert.Empty(t, recorder.Events(), "telemetry should not be recorded for disabled commands")
	})
}

func TestRecordTelemetryForSubcommands(t *testing.T) {
	t.Parallel()

	// Given a command tree instrumented from its root
	recorder := &telemetry.EventRecorderSpy{}
	root := &cobra.Command{Use: "gh"}
	parent := &cobra.Command{Use: "pr"}
	child := &cobra.Command{
		Use:  "list",
		RunE: func(cmd *cobra.Command, args []string) error { return nil },
	}
	root.AddCommand(parent)
	root.AddCommand(&cobra.Command{
		Use:  "version",
		RunE: func(cmd *cobra.Command, args []string) error { return nil },
	})
	parent.AddCommand(child)
	root.SetArgs([]string{"pr", "list"})
	cmdutil.RecordTelemetryForSubcommands(root, recorder)

	// When Cobra executes a nested command
	_, err := root.ExecuteC()

	// Then only the invoked descendant is recorded
	require.NoError(t, err)
	events := recorder.Events()
	require.Len(t, events, 1)
	assert.Equal(t, "command_invocation", events[0].Type)
	assert.Equal(t, "gh pr list", events[0].Dimensions["command"])
}
