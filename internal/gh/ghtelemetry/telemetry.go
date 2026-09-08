package ghtelemetry

type Dimensions map[string]string

type Measures map[string]int64

type Event struct {
	Type       string
	Dimensions Dimensions
	Measures   Measures
}

// PendingEvent accepts additional facts until its invocation finishes.
// Setters copy their input and have no effect after completion.
type PendingEvent interface {
	SetDimensions(Dimensions)
	SetMeasures(Measures)
}

type Disabler interface {
	Disable()
}

// EventRecorder produces complete or in-progress events.
type EventRecorder interface {
	Record(event Event)
	// BeginEvent records initial facts that can be updated until invocation completion.
	BeginEvent(Event) PendingEvent
}

// InvocationRecorder produces events and controls invocation-wide reporting policy.
type InvocationRecorder interface {
	EventRecorder
	Disabler
	SetSampleRate(rate int)
}

// Invocation owns the lifetime of telemetry collection for a command execution.
type Invocation interface {
	InvocationRecorder
	Finish()
}

const SAMPLE_ALL = 100
