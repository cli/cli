package ghtelemetry

type Dimensions map[string]string

type Measures map[string]int64

type Event struct {
	Type       string
	Dimensions Dimensions
	Measures   Measures
}

// RecordOptions holds configuration for a single Record call.
type RecordOptions struct {
	IncludeCommonDimensions bool
}

// RecordOption configures how an event is recorded.
type RecordOption func(*RecordOptions)

// IncludeCommonDimensions returns a RecordOption that causes common dimensions
// (device_id, invocation_id, os, architecture, etc.) to be merged into the
// event at flush time. Without this option, events only carry their own
// event-specific dimensions plus a timestamp and sample_rate.
func IncludeCommonDimensions() RecordOption {
	return func(o *RecordOptions) {
		o.IncludeCommonDimensions = true
	}
}

type Disabler interface {
	Disable()
}

type EventRecorder interface {
	Record(event Event, opts ...RecordOption)
	Disabler
}

type CommandRecorder interface {
	EventRecorder
	SetSampleRate(rate int)
}

type Service interface {
	CommandRecorder
	Flush()
}

const SAMPLE_ALL = 100
