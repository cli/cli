package telemetry

import (
	"encoding/binary"
	"maps"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/cli/cli/v2/internal/gh/ghtelemetry"
	"github.com/google/uuid"
)

var (
	_ ghtelemetry.Invocation = (*Invocation)(nil)
	_ ghtelemetry.Invocation = (*NoOpInvocation)(nil)
)

// Invocation owns telemetry facts and reporting policy for one command execution.
// Finish must run after command execution to send the completed payload.
type Invocation struct {
	mu               sync.Mutex
	send             func(SendTelemetryPayload)
	commonDimensions ghtelemetry.Dimensions
	sampleRate       int
	sampleBucket     byte
	events           []*invocationEvent
	disabled         bool
	finished         bool
}

type invocationEvent struct {
	event      ghtelemetry.Event
	recordedAt time.Time
}

type pendingEvent struct {
	invocation *Invocation
	recorded   *invocationEvent
}

type invocationOptions struct {
	additionalDimensions ghtelemetry.Dimensions
	sampleRate           int
}

type invocationOption func(*invocationOptions)

// WithAdditionalCommonDimensions sets dimensions shared by every invocation event.
func WithAdditionalCommonDimensions(dimensions ghtelemetry.Dimensions) invocationOption {
	return func(options *invocationOptions) {
		maps.Copy(options.additionalDimensions, dimensions)
	}
}

// WithSampleRate selects invocation-wide sampling. Rates 0 and 100 retain all
// events; rates between them select a percentage using the invocation ID.
func WithSampleRate(rate int) invocationOption {
	return func(options *invocationOptions) {
		options.sampleRate = rate
	}
}

// NewInvocation creates an invocation using send to deliver its completed payload.
func NewInvocation(send func(SendTelemetryPayload), opts ...invocationOption) *Invocation {
	options := invocationOptions{
		additionalDimensions: make(ghtelemetry.Dimensions),
	}
	for _, opt := range opts {
		opt(&options)
	}

	deviceID, err := deviceIDFunc()
	if err != nil {
		deviceID = "<unknown>"
	}
	invocationID := uuid.NewString()
	commonDimensions := ghtelemetry.Dimensions{
		"device_id":     deviceID,
		"invocation_id": invocationID,
		"os":            runtime.GOOS,
		"architecture":  runtime.GOARCH,
	}
	maps.Copy(commonDimensions, options.additionalDimensions)

	hash := uuid.NewSHA1(uuid.Nil, []byte(invocationID))
	sampleBucket := byte(binary.BigEndian.Uint32(hash[:4]) % 100)

	return &Invocation{
		send:             send,
		commonDimensions: commonDimensions,
		sampleRate:       options.sampleRate,
		sampleBucket:     sampleBucket,
	}
}

// Record copies a complete event into the invocation.
// Recording after Finish has no effect.
func (i *Invocation) Record(event ghtelemetry.Event) {
	i.mu.Lock()
	defer i.mu.Unlock()

	if i.finished {
		return
	}
	i.events = append(i.events, &invocationEvent{
		event:      cloneEvent(event),
		recordedAt: time.Now(),
	})
}

// BeginEvent copies an event's initial facts and returns a handle for adding
// facts until Finish. Events begun after Finish are not recorded.
func (i *Invocation) BeginEvent(event ghtelemetry.Event) ghtelemetry.PendingEvent {
	i.mu.Lock()
	defer i.mu.Unlock()

	if i.finished {
		return noOpPendingEvent{}
	}
	recorded := &invocationEvent{
		event:      cloneEvent(event),
		recordedAt: time.Now(),
	}
	i.events = append(i.events, recorded)
	return &pendingEvent{invocation: i, recorded: recorded}
}

func (p *pendingEvent) SetDimensions(dimensions ghtelemetry.Dimensions) {
	p.invocation.mu.Lock()
	defer p.invocation.mu.Unlock()

	if p.invocation.finished {
		return
	}
	if p.recorded.event.Dimensions == nil {
		p.recorded.event.Dimensions = make(ghtelemetry.Dimensions)
	}
	maps.Copy(p.recorded.event.Dimensions, dimensions)
}

func (p *pendingEvent) SetMeasures(measures ghtelemetry.Measures) {
	p.invocation.mu.Lock()
	defer p.invocation.mu.Unlock()

	if p.invocation.finished {
		return
	}
	if p.recorded.event.Measures == nil {
		p.recorded.event.Measures = make(ghtelemetry.Measures)
	}
	maps.Copy(p.recorded.event.Measures, measures)
}

// SetSampleRate selects the sampling policy for the whole invocation.
// Changes after Finish have no effect.
func (i *Invocation) SetSampleRate(rate int) {
	i.mu.Lock()
	defer i.mu.Unlock()

	if i.finished {
		return
	}
	i.sampleRate = rate
	i.commonDimensions["sample_rate"] = strconv.Itoa(rate)
}

// Disable suppresses all events in the invocation, including already recorded
// events. It must be called before Finish.
func (i *Invocation) Disable() {
	i.mu.Lock()
	defer i.mu.Unlock()

	i.disabled = true
}

// Finish snapshots the invocation once and sends its payload after releasing the lock.
// Sampling and telemetry eligibility apply to immediate and pending events alike.
func (i *Invocation) Finish() {
	i.mu.Lock()

	if i.finished {
		i.mu.Unlock()
		return
	}
	i.finished = true

	if i.sampleRate > 0 && i.sampleRate < 100 && int(i.sampleBucket) >= i.sampleRate {
		i.mu.Unlock()
		return
	}

	events := i.events
	if i.disabled {
		events = nil
	}

	// Keep an empty payload so log mode can explain that no telemetry will be sent.
	payload := SendTelemetryPayload{Events: make([]PayloadEvent, len(events))}
	for index, recorded := range events {
		dimensions := map[string]string{
			"timestamp": recorded.recordedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
		}
		maps.Copy(dimensions, i.commonDimensions)
		maps.Copy(dimensions, recorded.event.Dimensions)
		payload.Events[index] = PayloadEvent{
			Type:       recorded.event.Type,
			Dimensions: dimensions,
			Measures:   maps.Clone(recorded.event.Measures),
		}
	}
	i.mu.Unlock()

	i.send(payload)
}

func cloneEvent(event ghtelemetry.Event) ghtelemetry.Event {
	return ghtelemetry.Event{
		Type:       event.Type,
		Dimensions: maps.Clone(event.Dimensions),
		Measures:   maps.Clone(event.Measures),
	}
}

type noOpPendingEvent struct{}

func (noOpPendingEvent) SetDimensions(ghtelemetry.Dimensions) {}
func (noOpPendingEvent) SetMeasures(ghtelemetry.Measures)     {}

// NoOpInvocation discards telemetry when collection is disabled.
type NoOpInvocation struct{}

// Record discards the event.
func (*NoOpInvocation) Record(ghtelemetry.Event) {}

// BeginEvent returns an inert handle without retaining the event.
func (*NoOpInvocation) BeginEvent(ghtelemetry.Event) ghtelemetry.PendingEvent {
	return noOpPendingEvent{}
}

// Disable leaves telemetry disabled.
func (*NoOpInvocation) Disable() {}

// SetSampleRate leaves telemetry disabled.
func (*NoOpInvocation) SetSampleRate(int) {}

// Finish has no payload to complete.
func (*NoOpInvocation) Finish() {}
