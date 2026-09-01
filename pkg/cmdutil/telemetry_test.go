package cmdutil_test

import (
	"testing"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestIsTelemetryDisabled(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		want        bool
	}{
		{
			name:        "nil annotations",
			annotations: nil,
			want:        false,
		},
		{
			name:        "empty annotations",
			annotations: map[string]string{},
			want:        false,
		},
		{
			name:        "unrelated annotation",
			annotations: map[string]string{"other": "value"},
			want:        false,
		},
		{
			name:        "telemetry annotation set to disabled",
			annotations: map[string]string{"telemetry": "disabled"},
			want:        true,
		},
		{
			name:        "telemetry annotation set to another value",
			annotations: map[string]string{"telemetry": "enabled"},
			want:        false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Annotations: tt.annotations}
			assert.Equal(t, tt.want, cmdutil.IsTelemetryDisabled(cmd))
		})
	}
}

func TestDisableTelemetry(t *testing.T) {
	t.Run("adds the disabled annotation when annotations are nil", func(t *testing.T) {
		cmd := &cobra.Command{}
		assert.False(t, cmdutil.IsTelemetryDisabled(cmd))

		cmdutil.DisableTelemetry(cmd)

		assert.True(t, cmdutil.IsTelemetryDisabled(cmd))
		assert.Equal(t, "disabled", cmd.Annotations["telemetry"])
	})

	t.Run("preserves existing annotations", func(t *testing.T) {
		cmd := &cobra.Command{Annotations: map[string]string{"other": "value"}}

		cmdutil.DisableTelemetry(cmd)

		assert.True(t, cmdutil.IsTelemetryDisabled(cmd))
		assert.Equal(t, "value", cmd.Annotations["other"])
	})
}

func TestDisableTelemetryRecursively(t *testing.T) {
	t.Run("disables telemetry on the root and all descendants", func(t *testing.T) {
		root := &cobra.Command{Use: "root"}
		child := &cobra.Command{Use: "child"}
		grandchild := &cobra.Command{Use: "grandchild"}
		sibling := &cobra.Command{Use: "sibling"}

		child.AddCommand(grandchild)
		root.AddCommand(child, sibling)

		cmdutil.DisableTelemetryRecursively(root)

		assert.True(t, cmdutil.IsTelemetryDisabled(root), "root should also be disabled")
		assert.True(t, cmdutil.IsTelemetryDisabled(child))
		assert.True(t, cmdutil.IsTelemetryDisabled(grandchild))
		assert.True(t, cmdutil.IsTelemetryDisabled(sibling))
	})

	t.Run("leaf command is still disabled", func(t *testing.T) {
		cmd := &cobra.Command{Use: "leaf"}

		cmdutil.DisableTelemetryRecursively(cmd)

		assert.True(t, cmdutil.IsTelemetryDisabled(cmd))
	})
}
