package telemetry

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/cli/cli/v2/internal/gh/ghtelemetry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func stubStateDir(dir string) func() {
	orig := stateDirFunc
	stateDirFunc = func() string { return dir }
	return func() { stateDirFunc = orig }
}

func stubDeviceID(id string) func() {
	orig := deviceIDFunc
	deviceIDFunc = func() (string, error) { return id, nil }
	return func() { deviceIDFunc = orig }
}

func stubDeviceIDError(err error) func() {
	orig := deviceIDFunc
	deviceIDFunc = func() (string, error) { return "", err }
	return func() { deviceIDFunc = orig }
}

func stubLookupEnv(fn func(string) (string, bool)) func() {
	orig := lookupEnvFunc
	lookupEnvFunc = fn
	return func() { lookupEnvFunc = orig }
}

func TestGetOrCreateDeviceID(t *testing.T) {
	t.Run("creates new ID on first call", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Cleanup(stubStateDir(tmpDir))

		id, err := getOrCreateDeviceID()
		require.NoError(t, err)
		require.NotEmpty(t, id)

		data, err := os.ReadFile(filepath.Join(tmpDir, deviceIDFileName))
		require.NoError(t, err)
		assert.Equal(t, id, string(data))
	})

	t.Run("returns same ID on subsequent calls", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Cleanup(stubStateDir(tmpDir))

		id1, err := getOrCreateDeviceID()
		require.NoError(t, err)

		id2, err := getOrCreateDeviceID()
		require.NoError(t, err)

		assert.Equal(t, id1, id2)
	})

	t.Run("trims whitespace from stored ID", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Cleanup(stubStateDir(tmpDir))

		err := os.WriteFile(filepath.Join(tmpDir, deviceIDFileName), []byte("  some-device-id\n"), 0o600)
		require.NoError(t, err)

		id, err := getOrCreateDeviceID()
		require.NoError(t, err)
		assert.Equal(t, "some-device-id", id)
	})

	t.Run("returns error for non-ErrNotExist read failures", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Cleanup(stubStateDir(tmpDir))

		// Create device-id as a directory so ReadFile fails with a non-ErrNotExist error.
		err := os.Mkdir(filepath.Join(tmpDir, deviceIDFileName), 0o755)
		require.NoError(t, err)

		_, err = getOrCreateDeviceID()
		require.Error(t, err)
		assert.False(t, errors.Is(err, os.ErrNotExist))
	})

	t.Run("creates state directory if missing", func(t *testing.T) {
		tmpDir := t.TempDir()
		nestedDir := filepath.Join(tmpDir, "nested", "state")
		t.Cleanup(stubStateDir(nestedDir))

		id, err := getOrCreateDeviceID()
		require.NoError(t, err)
		require.NotEmpty(t, id)

		data, err := os.ReadFile(filepath.Join(nestedDir, deviceIDFileName))
		require.NoError(t, err)
		assert.Equal(t, id, string(data))
	})

	t.Run("concurrent callers converge on the same ID", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Cleanup(stubStateDir(tmpDir))

		const goroutines = 10
		ids := make([]string, goroutines)
		errs := make([]error, goroutines)
		var wg sync.WaitGroup
		wg.Add(goroutines)
		for i := range goroutines {
			go func() {
				defer wg.Done()
				ids[i], errs[i] = getOrCreateDeviceID()
			}()
		}
		wg.Wait()

		for i := range goroutines {
			require.NoError(t, errs[i])
		}
		for i := 1; i < goroutines; i++ {
			assert.Equal(t, ids[0], ids[i], "goroutine %d returned a different ID", i)
		}
	})
}

func TestParseTelemetryState(t *testing.T) {
	envSet := func(val string) func(string) (string, bool) {
		return func(string) (string, bool) { return val, true }
	}
	envUnset := func(string) (string, bool) { return "", false }

	// envMap allows setting multiple environment variables for testing DO_NOT_TRACK + GH_TELEMETRY interactions.
	envMap := func(m map[string]string) func(string) (string, bool) {
		return func(key string) (string, bool) {
			val, ok := m[key]
			return val, ok
		}
	}

	tests := []struct {
		name        string
		lookupEnv   func(string) (string, bool)
		configValue string
		want        TelemetryState
	}{
		{
			name:        "env unset, config empty string disables",
			lookupEnv:   envUnset,
			configValue: "",
			want:        Disabled,
		},
		{
			name:        "env unset, config enabled",
			lookupEnv:   envUnset,
			configValue: "enabled",
			want:        Enabled,
		},
		{
			name:        "env unset, config disabled",
			lookupEnv:   envUnset,
			configValue: "disabled",
			want:        Disabled,
		},
		{
			name:        "env unset, config log",
			lookupEnv:   envUnset,
			configValue: "log",
			want:        Logged,
		},
		{
			name:        "env unset, config false",
			lookupEnv:   envUnset,
			configValue: "false",
			want:        Disabled,
		},
		{
			name:        "env unset, config any truthy value",
			lookupEnv:   envUnset,
			configValue: "anything",
			want:        Enabled,
		},
		{
			name:        "env enabled takes precedence over config disabled",
			lookupEnv:   envSet("enabled"),
			configValue: "disabled",
			want:        Enabled,
		},
		{
			name:        "env disabled takes precedence over config enabled",
			lookupEnv:   envSet("disabled"),
			configValue: "enabled",
			want:        Disabled,
		},
		{
			name:        "env log takes precedence over config enabled",
			lookupEnv:   envSet("log"),
			configValue: "enabled",
			want:        Logged,
		},
		{
			name:        "env false disables",
			lookupEnv:   envSet("false"),
			configValue: "enabled",
			want:        Disabled,
		},
		{
			name:        "env empty string disables",
			lookupEnv:   envSet(""),
			configValue: "enabled",
			want:        Disabled,
		},
		{
			name:        "env any truthy value enables",
			lookupEnv:   envSet("yes"),
			configValue: "disabled",
			want:        Enabled,
		},
		{
			name:        "env FALSE (uppercase) disables",
			lookupEnv:   envSet("FALSE"),
			configValue: "enabled",
			want:        Disabled,
		},
		{
			name:        "env LOG (uppercase) logs",
			lookupEnv:   envSet("LOG"),
			configValue: "enabled",
			want:        Logged,
		},
		{
			name:        "env value with whitespace is trimmed",
			lookupEnv:   envSet("  false  "),
			configValue: "enabled",
			want:        Disabled,
		},
		{
			name:        "DO_NOT_TRACK=1 disables telemetry",
			lookupEnv:   envMap(map[string]string{"DO_NOT_TRACK": "1"}),
			configValue: "enabled",
			want:        Disabled,
		},
		{
			name:        "DO_NOT_TRACK=true disables telemetry",
			lookupEnv:   envMap(map[string]string{"DO_NOT_TRACK": "true"}),
			configValue: "enabled",
			want:        Disabled,
		},
		{
			name:        "DO_NOT_TRACK=TRUE disables telemetry (case insensitive)",
			lookupEnv:   envMap(map[string]string{"DO_NOT_TRACK": "TRUE"}),
			configValue: "enabled",
			want:        Disabled,
		},
		{
			name:        "DO_NOT_TRACK=0 does not disable telemetry",
			lookupEnv:   envMap(map[string]string{"DO_NOT_TRACK": "0"}),
			configValue: "enabled",
			want:        Enabled,
		},
		{
			name:        "DO_NOT_TRACK with whitespace is trimmed",
			lookupEnv:   envMap(map[string]string{"DO_NOT_TRACK": " 1 "}),
			configValue: "enabled",
			want:        Disabled,
		},
		{
			name:        "GH_TELEMETRY takes precedence over DO_NOT_TRACK",
			lookupEnv:   envMap(map[string]string{"GH_TELEMETRY": "enabled", "DO_NOT_TRACK": "1"}),
			configValue: "",
			want:        Enabled,
		},
		{
			name:        "DO_NOT_TRACK takes precedence over config",
			lookupEnv:   envMap(map[string]string{"DO_NOT_TRACK": "1"}),
			configValue: "log",
			want:        Disabled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(stubLookupEnv(tt.lookupEnv))
			got := ParseTelemetryState(tt.configValue)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNewInvocationLogModeFlushesToWriter(t *testing.T) {
	// Given an invocation with log delivery
	t.Cleanup(stubDeviceID("test-device"))
	var buf bytes.Buffer
	delivery := NewDelivery(LogFlusher(&buf, false))
	invocation := NewInvocation(delivery)

	// When the invocation finishes and delivery is flushed
	invocation.Record(ghtelemetry.Event{
		Type:       "test_event",
		Dimensions: map[string]string{"key": "value"},
	})
	invocation.Finish()
	delivery.Flush()

	// Then the writer receives the recorded event
	output := buf.String()
	assert.Contains(t, output, "Telemetry payload:")
	assert.Contains(t, output, "test_event")
	assert.Contains(t, output, `"key"`)
	assert.Contains(t, output, `"value"`)
}

func TestNewInvocationLogModeWithColorLogsToWriter(t *testing.T) {
	// Given an invocation with colored log delivery
	t.Cleanup(stubDeviceID("test-device"))
	var buf bytes.Buffer
	delivery := NewDelivery(LogFlusher(&buf, true))
	invocation := NewInvocation(delivery)

	// When the invocation finishes and delivery is flushed
	invocation.Record(ghtelemetry.Event{Type: "color_event"})
	invocation.Finish()
	delivery.Flush()

	// Then the writer receives the event with ANSI color codes
	output := buf.String()
	assert.Contains(t, output, "color_event")
	assert.Contains(t, output, "\033[", "expected ANSI escape sequences when color is enabled")
}

func TestLogFlusherWritesNoneMarkerForEmptyPayload(t *testing.T) {
	t.Run("no color", func(t *testing.T) {
		var buf bytes.Buffer
		LogFlusher(&buf, false)(SendTelemetryPayload{})
		assert.Equal(t, "Telemetry payload: none\n", buf.String())
	})

	t.Run("with color", func(t *testing.T) {
		var buf bytes.Buffer
		LogFlusher(&buf, true)(SendTelemetryPayload{})
		output := buf.String()
		assert.Contains(t, output, "Telemetry payload:")
		assert.Contains(t, output, "none")
		assert.Contains(t, output, "\x1b") // ANSI escape char for color codes
	})
}

func TestInvocationFinishesPendingEventsBeforeDelivery(t *testing.T) {
	t.Cleanup(stubDeviceID("test-device"))

	// Given an invocation whose attachment operations are not yet known
	var payloads []SendTelemetryPayload
	delivery := NewDelivery(func(payload SendTelemetryPayload) {
		payloads = append(payloads, payload)
	})
	invocation := NewInvocation(delivery)
	event := invocation.BeginEvent(ghtelemetry.Event{
		Type: "attachment_invocation",
		Measures: ghtelemetry.Measures{
			"attach_count":      2,
			"append_ops_count":  0,
			"replace_ops_count": 0,
		},
	})

	// When delivery is flushed before the invocation finishes
	delivery.Flush()
	require.Empty(t, payloads, "delivery must not finalize an unfinished invocation")
	event.SetMeasures(ghtelemetry.Measures{
		"append_ops_count":  1,
		"replace_ops_count": 1,
	})
	invocation.Finish()
	require.Empty(t, payloads, "completion must not send telemetry")
	event.SetMeasures(ghtelemetry.Measures{"append_ops_count": 99})
	delivery.Flush()

	// Then delivery contains the snapshot taken at invocation completion
	require.Len(t, payloads, 1)
	require.Len(t, payloads[0].Events, 1)
	assert.Equal(t, "attachment_invocation", payloads[0].Events[0].Type)
	assert.Equal(t, map[string]int64{
		"attach_count":      2,
		"append_ops_count":  1,
		"replace_ops_count": 1,
	}, payloads[0].Events[0].Measures)
}

func TestInvocationDeviceIDFallback(t *testing.T) {
	// Given device ID discovery fails
	t.Cleanup(stubDeviceIDError(errors.New("no device id")))
	var captured SendTelemetryPayload
	delivery := NewDelivery(func(p SendTelemetryPayload) { captured = p })
	invocation := NewInvocation(delivery)

	// When a recorded event is completed and delivered
	invocation.Record(ghtelemetry.Event{Type: "test"})
	invocation.Finish()
	delivery.Flush()

	// Then the payload identifies the device as unknown
	require.Len(t, captured.Events, 1)
	assert.Equal(t, "<unknown>", captured.Events[0].Dimensions["device_id"])
}

func TestInvocationFinish(t *testing.T) {
	t.Run("logs none when no events recorded", func(t *testing.T) {
		// Given an invocation without events and log delivery
		t.Cleanup(stubDeviceID("test-device"))
		var buf bytes.Buffer
		delivery := NewDelivery(LogFlusher(&buf, false))
		invocation := NewInvocation(delivery)

		// When the invocation finishes and delivery is flushed
		invocation.Finish()
		delivery.Flush()

		// Then log mode explains the absence of telemetry
		assert.Equal(t, "Telemetry payload: none\n", buf.String())
	})

	t.Run("delivers events with merged dimensions", func(t *testing.T) {
		// Given an invocation with common dimensions
		t.Cleanup(stubDeviceID("test-device"))
		var captured SendTelemetryPayload
		delivery := NewDelivery(func(p SendTelemetryPayload) { captured = p })
		invocation := NewInvocation(delivery, WithAdditionalCommonDimensions(ghtelemetry.Dimensions{"version": "2.45.0"}))

		// When an event with its own dimensions and measures is completed and delivered
		invocation.Record(ghtelemetry.Event{
			Type:       "command_invocation",
			Dimensions: map[string]string{"command": "gh pr list"},
			Measures:   map[string]int64{"duration_ms": 150},
		})
		invocation.Finish()
		delivery.Flush()

		// Then the payload includes both common and event-specific facts
		require.Len(t, captured.Events, 1)
		event := captured.Events[0]
		assert.Equal(t, "command_invocation", event.Type)
		assert.Equal(t, "gh pr list", event.Dimensions["command"])
		assert.Equal(t, "2.45.0", event.Dimensions["version"])
		assert.Equal(t, "test-device", event.Dimensions["device_id"])
		assert.NotEmpty(t, event.Dimensions["timestamp"])
		assert.NotEmpty(t, event.Dimensions["invocation_id"])
		assert.Equal(t, int64(150), event.Measures["duration_ms"])
	})

	t.Run("delivers multiple events", func(t *testing.T) {
		// Given an invocation with two recorded events
		t.Cleanup(stubDeviceID("test-device"))
		var captured SendTelemetryPayload
		delivery := NewDelivery(func(p SendTelemetryPayload) { captured = p })
		invocation := NewInvocation(delivery)
		invocation.Record(ghtelemetry.Event{Type: "event1"})
		invocation.Record(ghtelemetry.Event{Type: "event2"})

		// When the invocation finishes and delivery is flushed
		invocation.Finish()
		delivery.Flush()

		// Then both events are delivered in recording order
		require.Len(t, captured.Events, 2)
		assert.Equal(t, "event1", captured.Events[0].Type)
		assert.Equal(t, "event2", captured.Events[1].Type)
	})

	t.Run("is idempotent", func(t *testing.T) {
		// Given an invocation with a recorded event
		t.Cleanup(stubDeviceID("test-device"))
		var payloads []SendTelemetryPayload
		delivery := NewDelivery(func(p SendTelemetryPayload) { payloads = append(payloads, p) })
		invocation := NewInvocation(delivery)
		invocation.Record(ghtelemetry.Event{Type: "test"})

		// When completion and delivery are repeated
		invocation.Finish()
		delivery.Flush()
		invocation.Finish()
		delivery.Flush()
		invocation.Finish()
		delivery.Flush()

		// Then the recorded event is delivered exactly once
		require.Len(t, payloads, 1)
		require.Len(t, payloads[0].Events, 1)
		assert.Equal(t, "test", payloads[0].Events[0].Type)
	})

	t.Run("event dimensions override common dimensions", func(t *testing.T) {
		// Given common and event dimensions share a key
		t.Cleanup(stubDeviceID("test-device"))
		var captured SendTelemetryPayload
		delivery := NewDelivery(func(p SendTelemetryPayload) { captured = p })
		invocation := NewInvocation(delivery, WithAdditionalCommonDimensions(ghtelemetry.Dimensions{"shared": "common"}))
		invocation.Record(ghtelemetry.Event{
			Type:       "test",
			Dimensions: map[string]string{"shared": "event-level"},
		})

		// When the invocation finishes and delivery is flushed
		invocation.Finish()
		delivery.Flush()

		// Then the event dimension takes precedence
		require.Len(t, captured.Events, 1)
		assert.Equal(t, "event-level", captured.Events[0].Dimensions["shared"])
	})

	t.Run("timestamps reflect record time not completion or delivery time", func(t *testing.T) {
		t.Cleanup(stubDeviceID("test-device"))
		synctest.Test(t, func(t *testing.T) {
			// Given events recorded at distinct times
			var captured SendTelemetryPayload
			delivery := NewDelivery(func(p SendTelemetryPayload) { captured = p })
			invocation := NewInvocation(delivery)
			firstRecordedAt := time.Now()
			invocation.Record(ghtelemetry.Event{Type: "early"})
			time.Sleep(50 * time.Millisecond)
			secondRecordedAt := time.Now()
			invocation.Record(ghtelemetry.Event{Type: "late"})

			// When completion and delivery each happen later
			time.Sleep(time.Second)
			invocation.Finish()
			time.Sleep(time.Second)
			delivery.Flush()

			// Then each timestamp reflects when its event was recorded
			require.Len(t, captured.Events, 2)
			firstTimestamp, err := time.Parse("2006-01-02T15:04:05.000Z", captured.Events[0].Dimensions["timestamp"])
			require.NoError(t, err)
			secondTimestamp, err := time.Parse("2006-01-02T15:04:05.000Z", captured.Events[1].Dimensions["timestamp"])
			require.NoError(t, err)
			assert.WithinDuration(t, firstRecordedAt, firstTimestamp, 0)
			assert.WithinDuration(t, secondRecordedAt, secondTimestamp, 0)
		})
	})
}

func TestInvocationSampling(t *testing.T) {
	tests := []struct {
		name         string
		sampleRate   int
		sampleBucket byte
		wantPayloads int
	}{
		{
			name:         "sampleRate 0 sends all events",
			sampleRate:   0,
			sampleBucket: 99,
			wantPayloads: 1,
		},
		{
			name:         "sampleRate 100 sends all events regardless of bucket",
			sampleRate:   100,
			sampleBucket: 99,
			wantPayloads: 1,
		},
		{
			name:         "bucket below sampleRate sends events",
			sampleRate:   50,
			sampleBucket: 49,
			wantPayloads: 1,
		},
		{
			name:         "bucket at sampleRate drops events",
			sampleRate:   50,
			sampleBucket: 50,
			wantPayloads: 0,
		},
		{
			name:         "bucket above sampleRate drops events",
			sampleRate:   1,
			sampleBucket: 50,
			wantPayloads: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given a configured sample rate and a deterministic sampling bucket
			t.Cleanup(stubDeviceID("test-device"))
			var payloads []SendTelemetryPayload
			delivery := NewDelivery(func(p SendTelemetryPayload) { payloads = append(payloads, p) })
			invocation := NewInvocation(delivery, WithSampleRate(tt.sampleRate))
			// Fix the random bucket so sampling boundaries can be asserted through delivery.
			invocation.sampleBucket = tt.sampleBucket

			// When the invocation finishes and delivery is flushed
			invocation.Record(ghtelemetry.Event{Type: "test"})
			invocation.Finish()
			delivery.Flush()

			// Then only invocations selected by sampling are delivered
			require.Len(t, payloads, tt.wantPayloads)
			for _, payload := range payloads {
				require.Len(t, payload.Events, 1)
				assert.Equal(t, "test", payload.Events[0].Type)
			}
		})
	}
}

func TestInvocationSetSampleRate(t *testing.T) {
	t.Run("changes delivery eligibility", func(t *testing.T) {
		// Given an invocation that initially sends all events
		t.Cleanup(stubDeviceID("test-device"))
		var payloads []SendTelemetryPayload
		delivery := NewDelivery(func(p SendTelemetryPayload) { payloads = append(payloads, p) })
		invocation := NewInvocation(delivery, WithSampleRate(0))
		invocation.sampleBucket = 50

		// When its sample rate excludes the bucket before completion
		invocation.SetSampleRate(10)
		invocation.Record(ghtelemetry.Event{Type: "test"})
		invocation.Finish()
		delivery.Flush()

		// Then no payload is delivered
		assert.Empty(t, payloads)
	})

	t.Run("updates sample_rate dimension", func(t *testing.T) {
		// Given an invocation with an initial sample_rate dimension
		t.Cleanup(stubDeviceID("test-device"))
		var captured SendTelemetryPayload
		delivery := NewDelivery(func(p SendTelemetryPayload) { captured = p })
		invocation := NewInvocation(delivery,
			WithSampleRate(1),
			WithAdditionalCommonDimensions(ghtelemetry.Dimensions{"sample_rate": "1"}),
		)

		// When the rate changes before completion and delivery
		invocation.SetSampleRate(100)
		invocation.Record(ghtelemetry.Event{Type: "test"})
		invocation.Finish()
		delivery.Flush()

		// Then the payload describes the effective sample rate
		require.Len(t, captured.Events, 1)
		assert.Equal(t, "100", captured.Events[0].Dimensions["sample_rate"])
	})
}

func TestWithAdditionalCommonDimensions(t *testing.T) {
	// Given an invocation constructed with additional common dimensions
	t.Cleanup(stubDeviceID("test-device"))
	var captured SendTelemetryPayload
	delivery := NewDelivery(func(p SendTelemetryPayload) { captured = p })
	invocation := NewInvocation(
		delivery,
		WithAdditionalCommonDimensions(ghtelemetry.Dimensions{
			"version": "2.45.0",
			"agent":   "none",
		}),
	)

	// When a recorded event is completed and delivered
	invocation.Record(ghtelemetry.Event{Type: "test"})
	invocation.Finish()
	delivery.Flush()

	// Then both additional and standard dimensions are present
	require.Len(t, captured.Events, 1)
	assert.Equal(t, "2.45.0", captured.Events[0].Dimensions["version"])
	assert.Equal(t, "none", captured.Events[0].Dimensions["agent"])
	assert.Equal(t, "test-device", captured.Events[0].Dimensions["device_id"])
	assert.NotEmpty(t, captured.Events[0].Dimensions["invocation_id"])
	assert.NotEmpty(t, captured.Events[0].Dimensions["os"])
	assert.NotEmpty(t, captured.Events[0].Dimensions["architecture"])
}

func TestInvocationDisable(t *testing.T) {
	t.Run("drops recorded events from delivered payload", func(t *testing.T) {
		// Given an invocation with a recorded event
		t.Cleanup(stubDeviceID("test-device"))
		var payloads []SendTelemetryPayload
		delivery := NewDelivery(func(p SendTelemetryPayload) { payloads = append(payloads, p) })
		invocation := NewInvocation(delivery)
		invocation.Record(ghtelemetry.Event{Type: "test"})

		// When telemetry is disabled before completion and delivery
		invocation.Disable()
		invocation.Finish()
		delivery.Flush()

		// Then an empty payload is delivered so log mode can surface the absence
		require.Len(t, payloads, 1)
		assert.Empty(t, payloads[0].Events, "recorded events should be dropped after Disable()")
	})

	t.Run("drops events even with multiple recorded events", func(t *testing.T) {
		// Given an invocation with multiple recorded events
		t.Cleanup(stubDeviceID("test-device"))
		var payloads []SendTelemetryPayload
		delivery := NewDelivery(func(p SendTelemetryPayload) { payloads = append(payloads, p) })
		invocation := NewInvocation(delivery)
		invocation.Record(ghtelemetry.Event{Type: "event1"})
		invocation.Record(ghtelemetry.Event{Type: "event2"})
		invocation.Record(ghtelemetry.Event{Type: "event3"})

		// When telemetry is disabled before completion and delivery
		invocation.Disable()
		invocation.Finish()
		delivery.Flush()

		// Then none of the recorded events appear in the delivered payload
		require.Len(t, payloads, 1)
		assert.Empty(t, payloads[0].Events, "recorded events should be dropped after Disable()")
	})

	t.Run("can be called before any events are recorded", func(t *testing.T) {
		// Given an invocation disabled before any events are recorded
		t.Cleanup(stubDeviceID("test-device"))
		var payloads []SendTelemetryPayload
		delivery := NewDelivery(func(p SendTelemetryPayload) { payloads = append(payloads, p) })
		invocation := NewInvocation(delivery)
		invocation.Disable()

		// When an event is recorded and the invocation completes
		invocation.Record(ghtelemetry.Event{Type: "test"})
		invocation.Finish()
		delivery.Flush()

		// Then the later event is excluded from the delivered payload
		require.Len(t, payloads, 1)
		assert.Empty(t, payloads[0].Events, "events recorded after Disable() should be dropped")
	})
}

func TestNoOpInvocation(t *testing.T) {
	invocation := &NoOpInvocation{}
	// All methods should be safe to call without panicking
	invocation.Record(ghtelemetry.Event{Type: "test"})
	event := invocation.BeginEvent(ghtelemetry.Event{Type: "pending"})
	event.SetDimensions(ghtelemetry.Dimensions{"key": "value"})
	event.SetMeasures(ghtelemetry.Measures{"count": 1})
	invocation.Disable()
	invocation.SetSampleRate(50)
	invocation.Finish()
}

func TestSpawnSendTelemetryRejectsOversizedPayload(t *testing.T) {
	// Build a payload larger than maxPayloadSize (16KB)
	largeDimensions := map[string]string{
		"data": strings.Repeat("x", maxPayloadSize),
	}
	payload := SendTelemetryPayload{
		Events: []PayloadEvent{
			{Type: "test", Dimensions: largeDimensions},
		},
	}

	// This should not panic or spawn a process - it silently returns.
	// We can't easily assert the subprocess wasn't started, but we verify
	// the function doesn't crash.
	SpawnSendTelemetry("/nonexistent/binary", payload)
}
