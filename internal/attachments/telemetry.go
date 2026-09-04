package attachments

import (
	"github.com/cli/cli/v2/internal/gh/ghtelemetry"
	"github.com/spf13/cobra"
)

// InvocationTelemetry records telemetry for one command invocation using
// attachments.
type InvocationTelemetry struct {
	flag     *Flag
	recorder ghtelemetry.CommandRecorder
	event    *ghtelemetry.Event
}

// NewInvocationTelemetry creates attachment telemetry for flag.
func NewInvocationTelemetry(flag *Flag, recorder ghtelemetry.CommandRecorder) *InvocationTelemetry {
	return &InvocationTelemetry{
		flag:     flag,
		recorder: recorder,
	}
}

// WrapArgs starts attachment telemetry before argument validation.
func (t *InvocationTelemetry) WrapArgs(validate cobra.PositionalArgs) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		t.start(cmd.CommandPath())
		return validate(cmd, args)
	}
}

func (t *InvocationTelemetry) start(command string) {
	if t == nil {
		return
	}

	t.event = nil
	if t.recorder == nil || t.flag == nil || !t.flag.Changed() {
		return
	}

	event := &ghtelemetry.Event{
		Type: "attachment_invocation",
		Dimensions: ghtelemetry.Dimensions{
			"command": command,
		},
		Measures: ghtelemetry.Measures{
			"attach_count":      int64(len(t.flag.values)),
			"append_ops_count":  0,
			"replace_ops_count": 0,
		},
	}
	t.event = event
	t.recorder.SetSampleRate(ghtelemetry.SAMPLE_ALL)
	t.recorder.RecordDeferred(func() ghtelemetry.Event {
		return *event
	})
}

// RecordOperations adds successful markdown operations to the invocation.
func (t *InvocationTelemetry) RecordOperations(result UploadResult) {
	if t == nil || t.event == nil {
		return
	}

	t.event.Measures["append_ops_count"] = int64(result.AppendOperations)
	t.event.Measures["replace_ops_count"] = int64(result.ReplaceOperations)
}
