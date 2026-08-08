package cmdutil

import (
	"slices"
	"strings"

	"github.com/cli/cli/v2/internal/gh/ghtelemetry"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func RecordTelemetry(cmd *cobra.Command, telemetry ghtelemetry.EventRecorder) {
	if isTelemetryDisabled(cmd) {
		return
	}

	if cmd.RunE == nil {
		return
	}

	currentRunE := cmd.RunE
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		runErr := currentRunE(cmd, args)

		var flags []string
		cmd.Flags().Visit(func(f *pflag.Flag) {
			flags = append(flags, f.Name)
		})
		slices.Sort(flags)

		telemetry.Record(ghtelemetry.Event{
			Type: "command_invocation",
			Dimensions: map[string]string{
				"command": cmd.CommandPath(),
				"flags":   strings.Join(flags, ","),
			},
		})

		return runErr
	}
}

func RecordTelemetryForSubcommands(cmd *cobra.Command, telemetry ghtelemetry.EventRecorder) {
	for _, c := range cmd.Commands() {
		RecordTelemetry(c, telemetry)
		RecordTelemetryForSubcommands(c, telemetry)
	}
}

func DisableTelemetry(cmd *cobra.Command) {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations["telemetry"] = "disabled"
}

func DisableTelemetryForSubcommands(cmd *cobra.Command) {
	for _, c := range cmd.Commands() {
		DisableTelemetry(c)
		DisableTelemetryForSubcommands(c)
	}
}

func isTelemetryDisabled(cmd *cobra.Command) bool {
	return cmd.Annotations["telemetry"] == "disabled"
}
