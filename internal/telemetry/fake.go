package telemetry

import "github.com/cli/cli/v2/internal/gh/ghtelemetry"

type EventRecorderSpy struct {
	Events  []ghtelemetry.Event
	Options []ghtelemetry.RecordOptions
}

func (r *EventRecorderSpy) Record(event ghtelemetry.Event, opts ...ghtelemetry.RecordOption) {
	r.Events = append(r.Events, event)
	var options ghtelemetry.RecordOptions
	for _, opt := range opts {
		opt(&options)
	}
	r.Options = append(r.Options, options)
}

func (r *EventRecorderSpy) Disable() {}

func (r *EventRecorderSpy) Flush() {}

// CommandRecorderSpy is a test double for ghtelemetry.CommandRecorder.
// It captures recorded events and the most recent SetSampleRate call so tests can
// assert on the sampling behavior commands attempt to configure.
type CommandRecorderSpy struct {
	Events         []ghtelemetry.Event
	Options        []ghtelemetry.RecordOptions
	LastSampleRate int
}

func (r *CommandRecorderSpy) Record(event ghtelemetry.Event, opts ...ghtelemetry.RecordOption) {
	r.Events = append(r.Events, event)
	var options ghtelemetry.RecordOptions
	for _, opt := range opts {
		opt(&options)
	}
	r.Options = append(r.Options, options)
}

func (r *CommandRecorderSpy) Disable() {}

func (r *CommandRecorderSpy) SetSampleRate(rate int) {
	r.LastSampleRate = rate
}

func (r *CommandRecorderSpy) Flush() {}
