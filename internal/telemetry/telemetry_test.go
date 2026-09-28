package telemetry

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

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

func TestNewServiceLogModeFlushesToWriter(t *testing.T) {
	// Given an invocation with log delivery
	t.Cleanup(stubDeviceID("test-device"))
	var buf bytes.Buffer
	service := NewService(LogFlusher(&buf, false))

	// When the invocation finishes
	service.Record(ghtelemetry.Event{
		Type:       "test_event",
		Dimensions: map[string]string{"key": "value"},
	})
	service.Finish()

	// Then the writer receives the recorded event
	output := buf.String()
	assert.Contains(t, output, "Telemetry payload:")
	assert.Contains(t, output, "test_event")
	assert.Contains(t, output, `"key"`)
	assert.Contains(t, output, `"value"`)
}

func TestNewServiceLogModeWithColorLogsToWriter(t *testing.T) {
	// Given an invocation with colored log delivery
	t.Cleanup(stubDeviceID("test-device"))
	var buf bytes.Buffer
	service := NewService(LogFlusher(&buf, true))

	// When the invocation finishes
	service.Record(ghtelemetry.Event{Type: "color_event"})
	service.Finish()

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

func TestServiceDeviceIDFallback(t *testing.T) {
	// Given device ID discovery fails
	t.Cleanup(stubDeviceIDError(errors.New("no device id")))
	var captured SendTelemetryPayload
	service := NewService(func(p SendTelemetryPayload) { captured = p })

	// When a recorded event is completed and delivered
	service.Record(ghtelemetry.Event{Type: "test"})
	service.Finish()

	// Then the payload identifies the device as unknown
	require.Len(t, captured.Events, 1)
	assert.Equal(t, "<unknown>", captured.Events[0].Dimensions["device_id"])
}

func TestServiceFinish(t *testing.T) {
	t.Run("logs none when no events recorded", func(t *testing.T) {
		// Given an invocation without events and log delivery
		t.Cleanup(stubDeviceID("test-device"))
		var buf bytes.Buffer
		service := NewService(LogFlusher(&buf, false))

		// When the invocation finishes
		service.Finish()

		// Then log mode explains the absence of telemetry
		assert.Equal(t, "Telemetry payload: none\n", buf.String())
	})

	t.Run("delivers events with merged dimensions", func(t *testing.T) {
		// Given an invocation with common dimensions
		t.Cleanup(stubDeviceID("test-device"))
		var captured SendTelemetryPayload
		service := NewService(func(p SendTelemetryPayload) { captured = p },
			WithAdditionalCommonDimensions(ghtelemetry.Dimensions{
				"version": "2.45.0",
				"agent":   "none",
			}),
		)

		// When an event with its own dimensions and measures is completed and delivered
		service.Record(ghtelemetry.Event{
			Type:       "command_invocation",
			Dimensions: map[string]string{"command": "gh pr list"},
			Measures:   map[string]int64{"duration_ms": 150},
		})
		service.Finish()

		// Then the payload includes both common and event-specific facts
		require.Len(t, captured.Events, 1)
		event := captured.Events[0]
		assert.Equal(t, "command_invocation", event.Type)
		assert.Equal(t, "gh pr list", event.Dimensions["command"])
		assert.Equal(t, "2.45.0", event.Dimensions["version"])
		assert.Equal(t, "none", event.Dimensions["agent"])
		assert.Equal(t, "test-device", event.Dimensions["device_id"])
		assert.NotEmpty(t, event.Dimensions["timestamp"])
		assert.NotEmpty(t, event.Dimensions["invocation_id"])
		assert.NotEmpty(t, event.Dimensions["os"])
		assert.NotEmpty(t, event.Dimensions["architecture"])
		assert.Equal(t, int64(150), event.Measures["duration_ms"])
	})

	t.Run("event dimensions override common dimensions", func(t *testing.T) {
		// Given common and event dimensions share a key
		t.Cleanup(stubDeviceID("test-device"))
		var captured SendTelemetryPayload
		service := NewService(func(p SendTelemetryPayload) { captured = p }, WithAdditionalCommonDimensions(ghtelemetry.Dimensions{"shared": "common"}))
		service.Record(ghtelemetry.Event{
			Type:       "test",
			Dimensions: map[string]string{"shared": "event-level"},
		})

		// When the invocation finishes
		service.Finish()

		// Then the event dimension takes precedence
		require.Len(t, captured.Events, 1)
		assert.Equal(t, "event-level", captured.Events[0].Dimensions["shared"])
	})
}

func TestServiceSampling(t *testing.T) {
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
			svc := NewService(func(p SendTelemetryPayload) { payloads = append(payloads, p) }, WithSampleRate(tt.sampleRate))
			// Fix the random bucket so sampling boundaries can be asserted through delivery.
			svc.(*service).sampleBucket = tt.sampleBucket
			svc.Record(ghtelemetry.Event{Type: "test"})

			// When the invocation finishes
			svc.Finish()

			// Then the sampling policy determines whether a payload is delivered
			assert.Len(t, payloads, tt.wantPayloads)
		})
	}
}

func TestServiceReducingSampleRateExcludesInvocation(t *testing.T) {
	// Given an invocation that initially sends all events
	t.Cleanup(stubDeviceID("test-device"))
	var payloads []SendTelemetryPayload
	svc := NewService(func(p SendTelemetryPayload) { payloads = append(payloads, p) }, WithSampleRate(0))
	svc.(*service).sampleBucket = 50

	// When its sample rate excludes the bucket before completion
	svc.SetSampleRate(10)
	svc.Record(ghtelemetry.Event{Type: "test"})
	svc.Finish()

	// Then no payload is delivered
	assert.Empty(t, payloads)
}

func TestServiceDisabledBeforeRecordingDropsLaterEvents(t *testing.T) {
	// Given an invocation disabled before any events are recorded
	t.Cleanup(stubDeviceID("test-device"))
	var payloads []SendTelemetryPayload
	service := NewService(func(p SendTelemetryPayload) { payloads = append(payloads, p) })
	service.Disable()

	// When immediate and pending events are recorded and the invocation completes
	service.Record(ghtelemetry.Event{Type: "completed_step"})
	pending := service.Begin(ghtelemetry.Event{Type: "attachment_invocation"})
	pending.UpsertMeasures(ghtelemetry.Measures{"attach_count": 2})
	service.Finish()

	// Then the later events are excluded from the delivered payload
	require.Len(t, payloads, 1)
	assert.Empty(t, payloads[0].Events, "events recorded after Disable() should be dropped")
}

func TestNoOpService(t *testing.T) {
	service := &NoOpService{}
	// All methods should be safe to call without panicking
	service.Record(ghtelemetry.Event{Type: "test"})
	event := service.Begin(ghtelemetry.Event{Type: "pending"})
	event.UpsertDimensions(ghtelemetry.Dimensions{"key": "value"})
	event.UpsertMeasures(ghtelemetry.Measures{"count": 1})
	service.Disable()
	service.SetSampleRate(50)
	service.Finish()
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
