package attachments

import "github.com/cli/cli/v2/internal/gh/ghtelemetry"

// BeginTelemetry records attachment usage before validation and promotes full sampling.
// It returns nil if the flag was not passed or the flag or recorder is absent.
func BeginTelemetry(flag *Flag, recorder ghtelemetry.InvocationRecorder, command string) ghtelemetry.PendingEvent {
	if recorder == nil || flag == nil || !flag.Changed() {
		return nil
	}

	recorder.SetSampleRate(ghtelemetry.SAMPLE_ALL)
	return recorder.Begin(ghtelemetry.Event{
		Type: "attachment_invocation",
		Dimensions: ghtelemetry.Dimensions{
			"command": command,
		},
		Measures: ghtelemetry.Measures{
			"attach_count":      int64(len(flag.values)),
			"append_ops_count":  0,
			"replace_ops_count": 0,
		},
	})
}

// RecordOperations upserts completed markdown operation counts, including partial results.
// A nil event means there is no attachment telemetry to update.
func RecordOperations(event ghtelemetry.PendingEvent, result UploadResult) {
	if event == nil {
		return
	}

	event.UpsertMeasures(ghtelemetry.Measures{
		"append_ops_count":  int64(result.AppendOperations),
		"replace_ops_count": int64(result.ReplaceOperations),
	})
}
