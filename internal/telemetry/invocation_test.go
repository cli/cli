package telemetry

import (
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/cli/cli/v2/internal/gh/ghtelemetry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInvocationCopiesFactsAtTheirRecordingTime(t *testing.T) {
	t.Cleanup(stubDeviceID("test-device"))

	synctest.Test(t, func(t *testing.T) {
		// Given a producer that reuses its event and update maps
		var payload SendTelemetryPayload
		invocation := NewInvocation(func(p SendTelemetryPayload) { payload = p })
		facts := ghtelemetry.Event{
			Type:       "command_invocation",
			Dimensions: ghtelemetry.Dimensions{"command": "gh issue create"},
			Measures:   ghtelemetry.Measures{"count": 1},
		}
		startedAt := time.Now()
		pending := invocation.BeginEvent(facts)
		time.Sleep(time.Second)
		facts.Type = "completed_step"
		invocation.Record(facts)

		// When the producer updates pending facts and later reuses those maps
		facts.Dimensions["command"] = "unrelated"
		facts.Measures["count"] = 99
		dimensions := ghtelemetry.Dimensions{"flags": "attach"}
		measures := ghtelemetry.Measures{"count": 2}
		pending.SetDimensions(dimensions)
		pending.SetMeasures(measures)
		dimensions["flags"] = "unrelated"
		measures["count"] = 99
		time.Sleep(time.Second)
		invocation.Finish()

		// Then event order, original timestamps, and independently owned facts survive
		require.Len(t, payload.Events, 2)
		first, second := payload.Events[0], payload.Events[1]
		assert.Equal(t, "command_invocation", first.Type)
		assert.Equal(t, "gh issue create", first.Dimensions["command"])
		assert.Equal(t, "attach", first.Dimensions["flags"])
		assert.Equal(t, int64(2), first.Measures["count"])
		assert.Equal(t, startedAt.UTC().Format("2006-01-02T15:04:05.000Z"), first.Dimensions["timestamp"])
		assert.Equal(t, "completed_step", second.Type)
		assert.Equal(t, "gh issue create", second.Dimensions["command"])
		assert.Equal(t, int64(1), second.Measures["count"])
		assert.Equal(t, startedAt.Add(time.Second).UTC().Format("2006-01-02T15:04:05.000Z"), second.Dimensions["timestamp"])
	})
}

func TestInvocationPromotesAllEventsBeforeCompletion(t *testing.T) {
	t.Cleanup(stubDeviceID("test-device"))

	// Given a command that discovers its full-sampling policy after recording facts
	var payload SendTelemetryPayload
	invocation := NewInvocation(func(p SendTelemetryPayload) { payload = p }, WithSampleRate(1))
	invocation.Record(ghtelemetry.Event{Type: "completed_step"})
	pending := invocation.BeginEvent(ghtelemetry.Event{Type: "attachment_invocation"})

	// When attachment usage promotes the invocation before it finishes
	invocation.SetSampleRate(ghtelemetry.SAMPLE_ALL)
	pending.SetMeasures(ghtelemetry.Measures{"attach_count": 2})
	invocation.Finish()

	// Then immediate and pending events share the promoted sampling policy
	require.Len(t, payload.Events, 2)
	assert.Equal(t, "completed_step", payload.Events[0].Type)
	assert.Equal(t, "attachment_invocation", payload.Events[1].Type)
	assert.Equal(t, "100", payload.Events[0].Dimensions["sample_rate"])
	assert.Equal(t, "100", payload.Events[1].Dimensions["sample_rate"])
	assert.Equal(t, payload.Events[0].Dimensions["invocation_id"], payload.Events[1].Dimensions["invocation_id"])
	assert.Equal(t, int64(2), payload.Events[1].Measures["attach_count"])
}

func TestInvocationDisablingOverridesPromotedPendingEvents(t *testing.T) {
	t.Cleanup(stubDeviceID("test-device"))

	// Given immediate and pending events in a fully sampled invocation
	var payloads []SendTelemetryPayload
	invocation := NewInvocation(func(p SendTelemetryPayload) { payloads = append(payloads, p) })
	invocation.Record(ghtelemetry.Event{Type: "completed_step"})
	pending := invocation.BeginEvent(ghtelemetry.Event{Type: "attachment_invocation"})
	invocation.SetSampleRate(ghtelemetry.SAMPLE_ALL)

	// When host discovery disables telemetry before completion
	invocation.Disable()
	pending.SetMeasures(ghtelemetry.Measures{"attach_count": 2})
	invocation.Record(ghtelemetry.Event{Type: "another_step"})
	invocation.Finish()

	// Then delivery receives only the empty payload used by log mode
	require.Len(t, payloads, 1)
	assert.Empty(t, payloads[0].Events)
}

func TestInvocationCompletionCannotBeReopened(t *testing.T) {
	t.Cleanup(stubDeviceID("test-device"))

	// Given a completed invocation with one pending event
	var payloads []SendTelemetryPayload
	invocation := NewInvocation(func(p SendTelemetryPayload) { payloads = append(payloads, p) })
	pending := invocation.BeginEvent(ghtelemetry.Event{
		Type:       "command_invocation",
		Dimensions: ghtelemetry.Dimensions{"command": "gh issue create"},
	})
	invocation.Finish()

	// When cleanup repeats or code holding an old handle attempts further recording
	pending.SetDimensions(ghtelemetry.Dimensions{"command": "changed"})
	invocation.Record(ghtelemetry.Event{Type: "too_late"})
	late := invocation.BeginEvent(ghtelemetry.Event{Type: "also_too_late"})
	late.SetDimensions(ghtelemetry.Dimensions{"command": "changed"})
	late.SetMeasures(ghtelemetry.Measures{"count": 1})
	invocation.Finish()
	invocation.Finish()

	// Then the original snapshot is delivered exactly once
	require.Len(t, payloads, 1)
	require.Len(t, payloads[0].Events, 1)
	assert.Equal(t, "command_invocation", payloads[0].Events[0].Type)
	assert.Equal(t, "gh issue create", payloads[0].Events[0].Dimensions["command"])
}

func TestInvocationCleanupDuringSendCannotChangePayload(t *testing.T) {
	t.Cleanup(stubDeviceID("test-device"))

	// Given a sender that is still working when command cleanup runs
	sendStarted := make(chan struct{})
	allowSend := make(chan struct{})
	var payloads []SendTelemetryPayload
	invocation := NewInvocation(func(payload SendTelemetryPayload) {
		close(sendStarted)
		<-allowSend
		payloads = append(payloads, payload)
	})
	pending := invocation.BeginEvent(ghtelemetry.Event{
		Type:       "attachment_invocation",
		Dimensions: ghtelemetry.Dimensions{"command": "gh issue create"},
		Measures:   ghtelemetry.Measures{"append_ops_count": 1},
	})

	// When cleanup updates an old handle and repeats completion during sending
	done := make(chan struct{})
	go func() {
		invocation.Finish()
		close(done)
	}()
	<-sendStarted
	cleanupDone := make(chan struct{})
	go func() {
		pending.SetDimensions(ghtelemetry.Dimensions{"command": "changed"})
		pending.SetMeasures(ghtelemetry.Measures{"append_ops_count": 99})
		invocation.Record(ghtelemetry.Event{Type: "too_late"})
		invocation.Finish()
		close(cleanupDone)
	}()

	// Then cleanup does not block on the sender or change its single snapshot
	select {
	case <-cleanupDone:
	case <-time.After(5 * time.Second):
		t.Error("command cleanup blocked while telemetry was being sent")
	}
	close(allowSend)
	<-done
	<-cleanupDone
	require.Len(t, payloads, 1)
	require.Len(t, payloads[0].Events, 1)
	assert.Equal(t, "attachment_invocation", payloads[0].Events[0].Type)
	assert.Equal(t, "gh issue create", payloads[0].Events[0].Dimensions["command"])
	assert.Equal(t, int64(1), payloads[0].Events[0].Measures["append_ops_count"])
}

func TestInvocationCollectsConcurrentFacts(t *testing.T) {
	t.Cleanup(stubDeviceID("test-device"))

	// Given two command activities contributing to the same pending event
	var payload SendTelemetryPayload
	invocation := NewInvocation(func(p SendTelemetryPayload) { payload = p })
	pending := invocation.BeginEvent(ghtelemetry.Event{Type: "attachment_invocation"})

	// When both activities finish before command completion
	var workers sync.WaitGroup
	workers.Go(func() {
		pending.SetDimensions(ghtelemetry.Dimensions{"command": "gh issue create"})
		pending.SetMeasures(ghtelemetry.Measures{"append_ops_count": 1})
	})
	workers.Go(func() {
		pending.SetDimensions(ghtelemetry.Dimensions{"flags": "attach"})
		pending.SetMeasures(ghtelemetry.Measures{"replace_ops_count": 2})
	})
	workers.Wait()
	invocation.Finish()

	// Then the completed event contains both activities' facts
	require.Len(t, payload.Events, 1)
	assert.Equal(t, "gh issue create", payload.Events[0].Dimensions["command"])
	assert.Equal(t, "attach", payload.Events[0].Dimensions["flags"])
	assert.Equal(t, map[string]int64{
		"append_ops_count":  1,
		"replace_ops_count": 2,
	}, payload.Events[0].Measures)
}
