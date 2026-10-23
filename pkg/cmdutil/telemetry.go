package cmdutil

import (
	"slices"
	"strings"

	"github.com/cli/cli/v2/internal/gh/ghtelemetry"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// RecordTelemetry instruments a command with an invocation-owned telemetry event.
func RecordTelemetry(cmd *cobra.Command, telemetry ghtelemetry.EventRecorder) {
	if isTelemetryDisabled(cmd) {
		return
	}

	if cmd.RunE == nil {
		return
	}

	var event ghtelemetry.PendingEvent
	currentArgs := cmd.Args
	cmd.Args = func(cmd *cobra.Command, args []string) error {
		event = telemetry.Begin(ghtelemetry.Event{
			Type: "command_invocation",
			Dimensions: ghtelemetry.Dimensions{
				"command": cmd.CommandPath(),
				"flags":   telemetryFlags(cmd),
			},
		})
		if currentArgs != nil {
			return currentArgs(cmd, args)
		}
		return nil
	}

	currentRunE := cmd.RunE
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		runErr := currentRunE(cmd, args)
		// Commands with DisableFlagParsing may parse their flags inside RunE.
		event.UpsertDimensions(ghtelemetry.Dimensions{"flags": telemetryFlags(cmd)})
		return runErr
	}
}

func telemetryFlags(cmd *cobra.Command) string {
	var flags []string
	cmd.Flags().Visit(func(f *pflag.Flag) {
		flags = append(flags, f.Name)
	})
	slices.Sort(flags)
	return strings.Join(flags, ",")
}

// RecordTelemetryForSubcommands instruments all descendants of a command.
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
