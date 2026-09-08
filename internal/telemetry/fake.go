package telemetry

import (
	"maps"

	"github.com/cli/cli/v2/internal/gh/ghtelemetry"
)

type EventRecorderSpy struct {
	Events []ghtelemetry.Event
}

func (r *EventRecorderSpy) Record(event ghtelemetry.Event) {
	r.Events = append(r.Events, event)
}

func (r *EventRecorderSpy) Disable() {}

// CommandRecorderSpy is a test double for ghtelemetry.CommandRecorder.
// Finish exposes completed events. LastSampleRate captures the sampling policy
// commands attempt to configure.
type CommandRecorderSpy struct {
	Events         []ghtelemetry.Event
	LastSampleRate int
	events         []*ghtelemetry.Event
	finished       bool
}

func (r *CommandRecorderSpy) Record(event ghtelemetry.Event) {
	r.BeginEvent(event)
}

func (r *CommandRecorderSpy) Disable() {}

// BeginEvent captures initial facts and returns a handle for subsequent updates.
func (r *CommandRecorderSpy) BeginEvent(event ghtelemetry.Event) ghtelemetry.PendingEvent {
	if r.finished {
		return noOpPendingEvent{}
	}
	event = cloneEvent(event)
	r.events = append(r.events, &event)
	return &pendingEventSpy{recorder: r, event: &event}
}

func (r *CommandRecorderSpy) SetSampleRate(rate int) {
	r.LastSampleRate = rate
}

// Finish snapshots recorded facts into Events once.
func (r *CommandRecorderSpy) Finish() {
	if r.finished {
		return
	}
	r.finished = true
	for _, event := range r.events {
		r.Events = append(r.Events, cloneEvent(*event))
	}
	r.events = nil
}

type pendingEventSpy struct {
	recorder *CommandRecorderSpy
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
