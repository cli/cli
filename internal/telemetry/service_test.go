package telemetry

import (
	"sync"
	"testing"
	"time"

	"github.com/cli/cli/v2/internal/gh/ghtelemetry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceCopiesRecordedEvents(t *testing.T) {
	t.Cleanup(stubDeviceID("test-device"))

	// Given a recorded event
	var payload SendTelemetryPayload
	service := NewService(func(p SendTelemetryPayload) { payload = p })
	event := ghtelemetry.Event{
		Type:       "command_invocation",
		Dimensions: ghtelemetry.Dimensions{"command": "gh issue create"},
		Measures:   ghtelemetry.Measures{"count": 1},
	}
	service.Record(event)

	// When the original maps are mutated before delivery
	event.Dimensions["command"] = "changed"
	event.Measures["count"] = 2
	service.Finish()

	// Then the recorded values are unchanged
	require.Len(t, payload.Events, 1)
	assert.Equal(t, "gh issue create", payload.Events[0].Dimensions["command"])
	assert.Equal(t, int64(1), payload.Events[0].Measures["count"])
}

func TestServicePromotesAllEventsBeforeCompletion(t *testing.T) {
	t.Cleanup(stubDeviceID("test-device"))

	// Given events recorded under a sampling policy that would exclude them
	var payload SendTelemetryPayload
	svc := NewService(func(p SendTelemetryPayload) { payload = p }, WithSampleRate(1))
	svc.(*service).sampleBucket = 99
	svc.Record(ghtelemetry.Event{Type: "completed_step"})
	svc.Begin(ghtelemetry.Event{Type: "attachment_invocation"})

	// When sampling is promoted before completion
	svc.SetSampleRate(ghtelemetry.SAMPLE_ALL)
	svc.Finish()

	// Then both events are delivered
	assert.Len(t, payload.Events, 2)
}

func TestServiceDisablingOverridesPromotedPendingEvents(t *testing.T) {
	t.Cleanup(stubDeviceID("test-device"))

	// Given immediate and pending events in a fully sampled invocation
	var payloads []SendTelemetryPayload
	service := NewService(func(p SendTelemetryPayload) { payloads = append(payloads, p) })
	service.Record(ghtelemetry.Event{Type: "completed_step"})
	pending := service.Begin(ghtelemetry.Event{Type: "attachment_invocation"})
	service.SetSampleRate(ghtelemetry.SAMPLE_ALL)

	// When host discovery disables telemetry before completion
	service.Disable()
	pending.UpsertMeasures(ghtelemetry.Measures{"attach_count": 2})
	service.Record(ghtelemetry.Event{Type: "another_step"})
	service.Finish()

	// Then delivery receives only the empty payload used by log mode
	require.Len(t, payloads, 1)
	assert.Empty(t, payloads[0].Events)
}

func TestServiceFinishDeliversOnce(t *testing.T) {
	t.Cleanup(stubDeviceID("test-device"))

	// Given an invocation with a recorded event
	deliveries := 0
	service := NewService(func(SendTelemetryPayload) { deliveries++ })
	service.Record(ghtelemetry.Event{Type: "test"})

	// When completion is called twice
	service.Finish()
	service.Finish()

	// Then the payload is delivered only once
	assert.Equal(t, 1, deliveries)
}

func TestServiceRecordingDoesNotWaitForDelivery(t *testing.T) {
	t.Cleanup(stubDeviceID("test-device"))

	// Given an invocation whose delivery is blocked
	sendStarted := make(chan struct{})
	allowSend := make(chan struct{})
	service := NewService(func(SendTelemetryPayload) {
		close(sendStarted)
		<-allowSend
	})
	service.Record(ghtelemetry.Event{Type: "test"})
	deliveryDone := make(chan struct{})
	go func() {
		service.Finish()
		close(deliveryDone)
	}()
	<-sendStarted

	// When another recording is attempted
	recordingDone := make(chan struct{})
	go func() {
		service.Record(ghtelemetry.Event{Type: "too_late"})
		close(recordingDone)
	}()

	// Then recording returns without waiting for delivery
	select {
	case <-recordingDone:
	case <-time.After(5 * time.Second):
		t.Error("recording blocked while telemetry was being sent")
	}
	close(allowSend)
	<-deliveryDone
	<-recordingDone
}

func TestServiceCollectsConcurrentFacts(t *testing.T) {
	t.Cleanup(stubDeviceID("test-device"))

	// Given two command activities contributing to the same pending event
	var payload SendTelemetryPayload
	service := NewService(func(p SendTelemetryPayload) { payload = p })
	pending := service.Begin(ghtelemetry.Event{Type: "attachment_invocation"})

	// When both activities finish before command completion
	var workers sync.WaitGroup
	workers.Go(func() {
		pending.UpsertDimensions(ghtelemetry.Dimensions{"command": "gh issue create"})
		pending.UpsertMeasures(ghtelemetry.Measures{"append_ops_count": 1})
	})
	workers.Go(func() {
		pending.UpsertDimensions(ghtelemetry.Dimensions{"flags": "attach"})
		pending.UpsertMeasures(ghtelemetry.Measures{"replace_ops_count": 2})
	})
	workers.Wait()
	service.Finish()

	// Then the completed event contains both activities' facts
	require.Len(t, payload.Events, 1)
	assert.Equal(t, "gh issue create", payload.Events[0].Dimensions["command"])
	assert.Equal(t, "attach", payload.Events[0].Dimensions["flags"])
	assert.Equal(t, map[string]int64{
		"append_ops_count":  1,
		"replace_ops_count": 2,
	}, payload.Events[0].Measures)
}
