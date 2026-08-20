package cmdutil

import (
	"github.com/spf13/cobra"
)

// DisableTelemetry marks the given command so that no command_invocation
// telemetry event is recorded when it is executed.
func DisableTelemetry(cmd *cobra.Command) {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations["telemetry"] = "disabled"
}

// DisableTelemetryRecursively marks the given command and all of its descendants
// as telemetry-disabled, so that no command_invocation event is recorded when
// any of them is executed.
func DisableTelemetryRecursively(cmd *cobra.Command) {
	DisableTelemetry(cmd)
	for _, c := range cmd.Commands() {
		DisableTelemetryRecursively(c)
	}
}

// IsTelemetryDisabled reports whether the given command has been marked as
// telemetry-disabled via DisableTelemetry or DisableTelemetryRecursively.
func IsTelemetryDisabled(cmd *cobra.Command) bool {
	return cmd.Annotations["telemetry"] == "disabled"
}
