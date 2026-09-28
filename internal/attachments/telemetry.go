package attachments

import "github.com/cli/cli/v2/internal/gh/ghtelemetry"

// TelemetryEvent tracks attachment-specific facts in a pending telemetry event.
// [ghtelemetry.Service.Finish] owns completion; later updates are ignored.
type TelemetryEvent struct {
	pendingEvent ghtelemetry.PendingEvent
}

// BeginTelemetry starts an attachment event at full sampling for the raw supplied count.
// A zero count returns nil without changing sampling. recorder is required;
// use a no-op service when telemetry is disabled.
// Call immediately before Flag.UserAssets so invalid and over-limit inputs count.
func BeginTelemetry(recorder ghtelemetry.InvocationRecorder, command string, attachCount int) *TelemetryEvent {
	if attachCount == 0 {
		return nil
	}

	recorder.SetSampleRate(ghtelemetry.SAMPLE_ALL)
	pendingEvent := recorder.Begin(ghtelemetry.Event{
		Type: "attachment_invocation",
		Dimensions: ghtelemetry.Dimensions{
			"command": command,
		},
		Measures: ghtelemetry.Measures{
			"attach_count":      int64(attachCount),
			"append_ops_count":  0,
			"replace_ops_count": 0,
		},
	})

	return &TelemetryEvent{
		pendingEvent: pendingEvent,
	}
}

// RecordOperations replaces, rather than adds to, the completed operation counts.
// Pass partial results before handling an upload error so successful work is retained.
// A nil event, returned by Begin for zero attachments, is a no-op.
func (e *TelemetryEvent) RecordOperations(result UploadResult) {
	if e == nil {
		return
	}

	e.pendingEvent.UpsertMeasures(ghtelemetry.Measures{
		"append_ops_count":  int64(result.AppendOperations),
		"replace_ops_count": int64(result.ReplaceOperations),
	})
}
