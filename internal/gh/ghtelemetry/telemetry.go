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

// APIRequest contains telemetry-safe metadata about a GitHub API request.
type APIRequest struct {
	RequestID string
}

// APIRequestRecorder records GitHub API requests for invocation correlation.
type APIRequestRecorder interface {
	RecordAPIRequest(request APIRequest)
}

// APIRequestTelemetry records GitHub API requests and disables telemetry when
// requests cross a host boundary where telemetry is not allowed.
type APIRequestTelemetry interface {
	APIRequestRecorder
	Disabler
}

type CommandRecorder interface {
	EventRecorder
	SetSampleRate(rate int)
}

type Service interface {
	CommandRecorder
	APIRequestTelemetry
	Flush()
}

const SAMPLE_ALL = 100
