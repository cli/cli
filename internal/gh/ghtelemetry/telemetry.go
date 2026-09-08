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

type EventRecorder interface {
	Record(event Event)
	Disabler
}

type CommandRecorder interface {
	EventRecorder
	// BeginEvent records initial facts that can be updated until invocation completion.
	BeginEvent(Event) PendingEvent
	SetSampleRate(rate int)
}

// Invocation collects telemetry throughout command execution and completes it once.
type Invocation interface {
	CommandRecorder
	Finish()
}

const SAMPLE_ALL = 100
