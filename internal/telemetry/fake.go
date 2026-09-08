package telemetry

import (
	"maps"

	"github.com/cli/cli/v2/internal/gh/ghtelemetry"
)

var (
	_ ghtelemetry.EventRecorder = (*EventRecorderSpy)(nil)
	_ ghtelemetry.Invocation    = (*InvocationRecorderSpy)(nil)
)

// EventRecorderSpy captures complete events immediately. Finish includes pending
// events in recording order and freezes their handles without requiring policy methods.
type EventRecorderSpy struct {
	Events   []ghtelemetry.Event
	events   []*ghtelemetry.Event
	finished bool
}

// Record captures a complete event without waiting for Finish.
func (r *EventRecorderSpy) Record(event ghtelemetry.Event) {
	if r.finished {
		return
	}
	r.BeginEvent(event)
	r.Events = append(r.Events, cloneEvent(event))
}

// BeginEvent captures initial facts and returns a handle for subsequent updates.
func (r *EventRecorderSpy) BeginEvent(event ghtelemetry.Event) ghtelemetry.PendingEvent {
	if r.finished {
		return noOpPendingEvent{}
	}
	event = cloneEvent(event)
	r.events = append(r.events, &event)
	return &pendingEventSpy{recorder: r, event: &event}
}

// Finish snapshots recorded facts into Events once.
func (r *EventRecorderSpy) Finish() {
	if r.finished {
		return
	}
	r.finished = true
	r.Events = nil
	for _, event := range r.events {
		r.Events = append(r.Events, cloneEvent(*event))
	}
	r.events = nil
}

// InvocationRecorderSpy adds invocation policy to EventRecorderSpy.
type InvocationRecorderSpy struct {
	EventRecorderSpy
	LastSampleRate int
}

// Disable leaves captured events available for assertions.
func (r *InvocationRecorderSpy) Disable() {}

// SetSampleRate captures the sampling policy requested by a command.
func (r *InvocationRecorderSpy) SetSampleRate(rate int) {
	r.LastSampleRate = rate
}

type pendingEventSpy struct {
	recorder *EventRecorderSpy
	event    *ghtelemetry.Event
}

func (p *pendingEventSpy) SetDimensions(dimensions ghtelemetry.Dimensions) {
	if p.recorder.finished {
		return
	}
	if p.event.Dimensions == nil {
		p.event.Dimensions = make(ghtelemetry.Dimensions)
	}
	maps.Copy(p.event.Dimensions, dimensions)
}

func (p *pendingEventSpy) SetMeasures(measures ghtelemetry.Measures) {
	if p.recorder.finished {
		return
	}
	if p.event.Measures == nil {
		p.event.Measures = make(ghtelemetry.Measures)
	}
	maps.Copy(p.event.Measures, measures)
}
