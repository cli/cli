package ghtelemetry

type Dimensions map[string]string

type Measures map[string]int64

type Event struct {
	Type       string
	Dimensions Dimensions
	Measures   Measures
}

type Disabler interface {
	Disable()
}

type EventRecorder interface {
	Record(event Event)
	Disabler
}

// RequestIDRecorder records GitHub server request IDs for invocation correlation.
type RequestIDRecorder interface {
	RecordRequestID(requestID string)
}

type CommandRecorder interface {
	EventRecorder
	SetSampleRate(rate int)
}

type Service interface {
	CommandRecorder
	RequestIDRecorder
	Flush()
}

const SAMPLE_ALL = 100
