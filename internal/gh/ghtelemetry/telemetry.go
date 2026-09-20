package ghtelemetry

type Dimensions map[string]string

type Measures map[string]int64

type Event struct {
	Type       string
	Dimensions Dimensions
	Measures   Measures
}

// PendingEvent accepts additional facts until its invocation finishes.
// Upserts copy supplied entries, inserting new keys and overwriting existing ones.
// Unspecified keys are unchanged. Calls after completion have no effect.
type PendingEvent interface {
	UpsertDimensions(Dimensions)
	UpsertMeasures(Measures)
}

type Disabler interface {
	Disable()
}

// EventRecorder produces complete or in-progress events.
type EventRecorder interface {
	Record(event Event)
	// Begin records initial facts that can be updated until invocation completion.
	Begin(Event) PendingEvent
}

// InvocationRecorder produces events and controls invocation-wide sampling.
type InvocationRecorder interface {
	EventRecorder
	SetSampleRate(rate int)
}

// Service collects telemetry for one command execution and sends it on Finish.
type Service interface {
	InvocationRecorder
	Disabler
	Finish()
}

const SAMPLE_ALL = 100
