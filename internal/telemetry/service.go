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
	_ ghtelemetry.Service = (*Service)(nil)
	_ ghtelemetry.Service = (*NoOpService)(nil)
)

// Service records telemetry facts and reporting policy for one command execution.
// Finish must run after command execution to send the completed payload.
type Service struct {
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
	service  *Service
	recorded *invocationEvent
}

type serviceOptions struct {
	additionalDimensions ghtelemetry.Dimensions
	sampleRate           int
}

type serviceOption func(*serviceOptions)

// WithAdditionalCommonDimensions sets dimensions shared by every invocation event.
func WithAdditionalCommonDimensions(dimensions ghtelemetry.Dimensions) serviceOption {
	return func(options *serviceOptions) {
		maps.Copy(options.additionalDimensions, dimensions)
	}
}

// WithSampleRate selects invocation-wide sampling. Rates 0 and 100 retain all
// events; rates between them select a percentage using the invocation ID.
func WithSampleRate(rate int) serviceOption {
	return func(options *serviceOptions) {
		options.sampleRate = rate
	}
}

// NewService creates a telemetry service using send to deliver its completed payload.
func NewService(send func(SendTelemetryPayload), opts ...serviceOption) *Service {
	options := serviceOptions{
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

	return &Service{
		send:             send,
		commonDimensions: commonDimensions,
		sampleRate:       options.sampleRate,
		sampleBucket:     sampleBucket,
	}
}

// Record copies a complete event into the service.
// Recording after Finish has no effect.
func (s *Service) Record(event ghtelemetry.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.finished {
		return
	}
	s.events = append(s.events, &invocationEvent{
		event:      cloneEvent(event),
		recordedAt: time.Now(),
	})
}

// BeginEvent copies an event's initial facts and returns a handle for adding
// facts until Finish. Events begun after Finish are not recorded.
func (s *Service) BeginEvent(event ghtelemetry.Event) ghtelemetry.PendingEvent {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.finished {
		return noOpPendingEvent{}
	}
	recorded := &invocationEvent{
		event:      cloneEvent(event),
		recordedAt: time.Now(),
	}
	s.events = append(s.events, recorded)
	return &pendingEvent{service: s, recorded: recorded}
}

func (p *pendingEvent) SetDimensions(dimensions ghtelemetry.Dimensions) {
	p.service.mu.Lock()
	defer p.service.mu.Unlock()

	if p.service.finished {
		return
	}
	if p.recorded.event.Dimensions == nil {
		p.recorded.event.Dimensions = make(ghtelemetry.Dimensions)
	}
	maps.Copy(p.recorded.event.Dimensions, dimensions)
}

func (p *pendingEvent) SetMeasures(measures ghtelemetry.Measures) {
	p.service.mu.Lock()
	defer p.service.mu.Unlock()

	if p.service.finished {
		return
	}
	if p.recorded.event.Measures == nil {
		p.recorded.event.Measures = make(ghtelemetry.Measures)
	}
	maps.Copy(p.recorded.event.Measures, measures)
}

// SetSampleRate selects the sampling policy for the whole invocation.
// Changes after Finish have no effect.
func (s *Service) SetSampleRate(rate int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.finished {
		return
	}
	s.sampleRate = rate
	s.commonDimensions["sample_rate"] = strconv.Itoa(rate)
}

// Disable suppresses all events in the invocation, including already recorded
// events. It must be called before Finish.
func (s *Service) Disable() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.disabled = true
}

// Finish snapshots the recorded events once and sends their payload after releasing the lock.
// Sampling and telemetry eligibility apply to immediate and pending events alike.
func (s *Service) Finish() {
	s.mu.Lock()

	if s.finished {
		s.mu.Unlock()
		return
	}
	s.finished = true

	if s.sampleRate > 0 && s.sampleRate < 100 && int(s.sampleBucket) >= s.sampleRate {
		s.mu.Unlock()
		return
	}

	events := s.events
	if s.disabled {
		events = nil
	}

	// Keep an empty payload so log mode can explain that no telemetry will be sent.
	payload := SendTelemetryPayload{Events: make([]PayloadEvent, len(events))}
	for index, recorded := range events {
		dimensions := map[string]string{
			"timestamp": recorded.recordedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
		}
		maps.Copy(dimensions, s.commonDimensions)
		maps.Copy(dimensions, recorded.event.Dimensions)
		payload.Events[index] = PayloadEvent{
			Type:       recorded.event.Type,
			Dimensions: dimensions,
			Measures:   maps.Clone(recorded.event.Measures),
		}
	}
	s.mu.Unlock()

	s.send(payload)
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

// NoOpService discards telemetry when collection is disabled.
type NoOpService struct{}

// Record discards the event.
func (*NoOpService) Record(ghtelemetry.Event) {}

// BeginEvent returns an inert handle without retaining the event.
func (*NoOpService) BeginEvent(ghtelemetry.Event) ghtelemetry.PendingEvent {
	return noOpPendingEvent{}
}

// Disable leaves telemetry disabled.
func (*NoOpService) Disable() {}

// SetSampleRate leaves telemetry disabled.
func (*NoOpService) SetSampleRate(int) {}

// Finish has no payload to complete.
func (*NoOpService) Finish() {}
