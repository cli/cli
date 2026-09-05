package telemetry

import "github.com/cli/cli/v2/internal/gh/ghtelemetry"

type EventRecorderSpy struct {
	Events []ghtelemetry.Event
}

func (r *EventRecorderSpy) Record(event ghtelemetry.Event) {
	r.Events = append(r.Events, event)
}

func (r *EventRecorderSpy) Disable() {}

func (r *EventRecorderSpy) Flush() {}

// CommandRecorderSpy is a test double for ghtelemetry.CommandRecorder.
// It captures recorded events and the most recent SetSampleRate call so tests can
// assert on the sampling behavior commands attempt to configure.
type CommandRecorderSpy struct {
	Events         []ghtelemetry.Event
	LastSampleRate int
	deferredEvents []func() ghtelemetry.Event
}

func (r *CommandRecorderSpy) Record(event ghtelemetry.Event) {
	r.Events = append(r.Events, event)
}

func (r *CommandRecorderSpy) Disable() {}

func (r *CommandRecorderSpy) RecordDeferred(event func() ghtelemetry.Event) {
	r.deferredEvents = append(r.deferredEvents, event)
}

func (r *CommandRecorderSpy) SetSampleRate(rate int) {
	r.LastSampleRate = rate
}

func (r *CommandRecorderSpy) Flush() {
	for _, event := range r.deferredEvents {
		r.Events = append(r.Events, event())
	}
	r.deferredEvents = nil
}
