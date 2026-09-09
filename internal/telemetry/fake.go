package telemetry

import (
	"maps"

	"github.com/cli/cli/v2/internal/gh/ghtelemetry"
)

var (
	_ ghtelemetry.EventRecorder      = (*EventRecorderSpy)(nil)
	_ ghtelemetry.InvocationRecorder = (*InvocationRecorderSpy)(nil)
)

// EventRecorderSpy captures recorded and pending events in recording order.
type EventRecorderSpy struct {
	events []*ghtelemetry.Event
}

// Record captures a complete event.
func (r *EventRecorderSpy) Record(event ghtelemetry.Event) {
	r.Begin(event)
}

// Begin captures initial facts and returns a handle for subsequent updates.
func (r *EventRecorderSpy) Begin(event ghtelemetry.Event) ghtelemetry.PendingEvent {
	event = cloneEvent(event)
	r.events = append(r.events, &event)
	return &pendingEventSpy{event: &event}
}

// Events returns copies of all captured events with their latest updates.
func (r *EventRecorderSpy) Events() []ghtelemetry.Event {
	var events []ghtelemetry.Event
	for _, event := range r.events {
		events = append(events, cloneEvent(*event))
	}
	return events
}

// InvocationRecorderSpy adds invocation sampling to EventRecorderSpy.
type InvocationRecorderSpy struct {
	EventRecorderSpy
	LastSampleRate int
}

// SetSampleRate captures the sampling policy requested by a command.
func (r *InvocationRecorderSpy) SetSampleRate(rate int) {
	r.LastSampleRate = rate
}

type pendingEventSpy struct {
	event *ghtelemetry.Event
}

func (p *pendingEventSpy) UpsertDimensions(dimensions ghtelemetry.Dimensions) {
	if p.event.Dimensions == nil {
		p.event.Dimensions = make(ghtelemetry.Dimensions)
	}
	maps.Copy(p.event.Dimensions, dimensions)
}

func (p *pendingEventSpy) UpsertMeasures(measures ghtelemetry.Measures) {
	if p.event.Measures == nil {
		p.event.Measures = make(ghtelemetry.Measures)
	}
	maps.Copy(p.event.Measures, measures)
}
