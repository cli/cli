package telemetry

import (
	"bytes"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cli/cli/v2/internal/gh/ghtelemetry"
	"github.com/google/uuid"
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

// newService is a test helper that constructs the internal service struct
// directly, bypassing the config/env parsing of NewService but still
// resolving common dimensions like device_id and invocation_id.
func newService(flusher func(SendTelemetryPayload), additionalDimensions ghtelemetry.Dimensions) *service {
	deviceID, err := deviceIDFunc()
	if err != nil {
		deviceID = "<unknown>"
	}

	commonDimensions := ghtelemetry.Dimensions{
		"device_id":     deviceID,
		"invocation_id": uuid.NewString(),
	}
	maps.Copy(commonDimensions, additionalDimensions)

	return &service{
		flush:            flusher,
		commonDimensions: commonDimensions,
	}
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
	t.Cleanup(stubDeviceID("test-device"))

	var buf bytes.Buffer
	svc := NewService(LogFlusher(&buf, false))

	svc.Record(ghtelemetry.Event{
		Type:       "test_event",
		Dimensions: map[string]string{"key": "value"},
	})
	svc.Flush()

	output := buf.String()
	assert.Contains(t, output, "Telemetry payload:")
	assert.Contains(t, output, "test_event")
	assert.Contains(t, output, `"key"`)
	assert.Contains(t, output, `"value"`)
}

func TestNewServiceLogModeWithColorLogsToWriter(t *testing.T) {
	t.Cleanup(stubDeviceID("test-device"))

	var buf bytes.Buffer
	svc := NewService(LogFlusher(&buf, true))

	svc.Record(ghtelemetry.Event{Type: "color_event"})
	svc.Flush()

	output := buf.String()
	assert.Contains(t, output, "color_event")
	// Verify ANSI color codes are present in the output
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
	t.Cleanup(stubDeviceIDError(errors.New("no device id")))

	var captured SendTelemetryPayload
	svc := newService(func(p SendTelemetryPayload) { captured = p }, nil)

	svc.Record(ghtelemetry.Event{Type: "test"})
	svc.Flush()

	require.Len(t, captured.Events, 1)
	assert.Equal(t, "<unknown>", captured.Events[0].Dimensions["device_id"])
}

func TestServiceFlush(t *testing.T) {
	t.Run("calls flusher with empty payload when no events recorded", func(t *testing.T) {
		t.Cleanup(stubDeviceID("test-device"))

		var captured SendTelemetryPayload
		called := false
		svc := newService(func(p SendTelemetryPayload) {
			called = true
			captured = p
		}, nil)
		svc.Flush()

		assert.True(t, called, "flusher should be called even with no events so log mode can surface the absence")
		assert.Empty(t, captured.Events, "payload should have no events")
	})

	t.Run("flushes events with merged dimensions", func(t *testing.T) {
		t.Cleanup(stubDeviceID("test-device"))

		var captured SendTelemetryPayload
		svc := newService(func(p SendTelemetryPayload) { captured = p }, ghtelemetry.Dimensions{"version": "2.45.0"})

		svc.Record(ghtelemetry.Event{
			Type:       "command_invocation",
			Dimensions: map[string]string{"command": "gh pr list"},
			Measures:   map[string]int64{"duration_ms": 150},
		})
		svc.Flush()

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

	t.Run("flushes multiple events", func(t *testing.T) {
		t.Cleanup(stubDeviceID("test-device"))

		var captured SendTelemetryPayload
		svc := newService(func(p SendTelemetryPayload) { captured = p }, nil)

		svc.Record(ghtelemetry.Event{Type: "event1"})
		svc.Record(ghtelemetry.Event{Type: "event2"})
		svc.Flush()

		require.Len(t, captured.Events, 2)
		assert.Equal(t, "event1", captured.Events[0].Type)
		assert.Equal(t, "event2", captured.Events[1].Type)
	})

	t.Run("is idempotent", func(t *testing.T) {
		t.Cleanup(stubDeviceID("test-device"))

		callCount := 0
		svc := newService(func(SendTelemetryPayload) { callCount++ }, nil)
		svc.Record(ghtelemetry.Event{Type: "test"})

		svc.Flush()
		svc.Flush()
		svc.Flush()

		assert.Equal(t, 1, callCount, "flusher should only be called once")
	})

	t.Run("event dimensions override common dimensions", func(t *testing.T) {
		t.Cleanup(stubDeviceID("test-device"))

		var captured SendTelemetryPayload
		svc := newService(func(p SendTelemetryPayload) { captured = p }, ghtelemetry.Dimensions{"shared": "common"})

		svc.Record(ghtelemetry.Event{
			Type:       "test",
			Dimensions: map[string]string{"shared": "event-level"},
		})
		svc.Flush()

		require.Len(t, captured.Events, 1)
		// Event dimensions are copied last via maps.Copy, so they override common
		assert.Equal(t, "event-level", captured.Events[0].Dimensions["shared"])
	})

	t.Run("timestamps reflect record time not flush time", func(t *testing.T) {
		t.Cleanup(stubDeviceID("test-device"))

		var captured SendTelemetryPayload
		svc := newService(func(p SendTelemetryPayload) { captured = p }, nil)

		svc.Record(ghtelemetry.Event{Type: "early"})
		time.Sleep(50 * time.Millisecond)
		svc.Record(ghtelemetry.Event{Type: "late"})
		svc.Flush()

		require.Len(t, captured.Events, 2)
		ts1 := captured.Events[0].Dimensions["timestamp"]
		ts2 := captured.Events[1].Dimensions["timestamp"]
		require.NotEmpty(t, ts1)
		require.NotEmpty(t, ts2)

		t1, err := time.Parse("2006-01-02T15:04:05.000Z", ts1)
		require.NoError(t, err)
		t2, err := time.Parse("2006-01-02T15:04:05.000Z", ts2)
		require.NoError(t, err)

		assert.True(t, t2.After(t1), "second event timestamp %s should be after first %s", ts2, ts1)
	})
}

func TestServiceSampling(t *testing.T) {
	t.Run("sampleRate 0 sends all events", func(t *testing.T) {
		t.Cleanup(stubDeviceID("test-device"))

		var captured SendTelemetryPayload
		svc := newService(func(p SendTelemetryPayload) { captured = p }, nil)
		svc.sampleRate = 0
		svc.sampleBucket = 99

		svc.Record(ghtelemetry.Event{Type: "test"})
		svc.Flush()

		require.Len(t, captured.Events, 1)
	})

	t.Run("sampleRate 100 sends all events regardless of bucket", func(t *testing.T) {
		t.Cleanup(stubDeviceID("test-device"))

		var captured SendTelemetryPayload
		svc := newService(func(p SendTelemetryPayload) { captured = p }, nil)
		svc.sampleRate = 100
		svc.sampleBucket = 99

		svc.Record(ghtelemetry.Event{Type: "test"})
		svc.Flush()

		require.Len(t, captured.Events, 1)
	})

	t.Run("bucket below sampleRate sends events", func(t *testing.T) {
		t.Cleanup(stubDeviceID("test-device"))

		var captured SendTelemetryPayload
		svc := newService(func(p SendTelemetryPayload) { captured = p }, nil)
		svc.sampleRate = 50
		svc.sampleBucket = 49 // below rate, should be included

		svc.Record(ghtelemetry.Event{Type: "test"})
		svc.Flush()

		require.Len(t, captured.Events, 1)
	})

	t.Run("bucket at sampleRate drops events", func(t *testing.T) {
		t.Cleanup(stubDeviceID("test-device"))

		called := false
		svc := newService(func(SendTelemetryPayload) { called = true }, nil)
		svc.sampleRate = 50
		svc.sampleBucket = 50 // at rate boundary, should be excluded

		svc.Record(ghtelemetry.Event{Type: "test"})
		svc.Flush()

		assert.False(t, called, "flusher should not be called when bucket >= sampleRate")
	})

	t.Run("bucket above sampleRate drops events", func(t *testing.T) {
		t.Cleanup(stubDeviceID("test-device"))

		called := false
		svc := newService(func(SendTelemetryPayload) { called = true }, nil)
		svc.sampleRate = 1
		svc.sampleBucket = 50

		svc.Record(ghtelemetry.Event{Type: "test"})
		svc.Flush()

		assert.False(t, called, "flusher should not be called when bucket >= sampleRate")
	})

	t.Run("SetSampleRate changes flush behavior", func(t *testing.T) {
		t.Cleanup(stubDeviceID("test-device"))

		called := false
		svc := newService(func(SendTelemetryPayload) { called = true }, nil)
		svc.sampleBucket = 50

		// Initially rate=0, which sends everything
		svc.SetSampleRate(10) // Now bucket=50 >= rate=10, should drop
		svc.Record(ghtelemetry.Event{Type: "test"})
		svc.Flush()

		assert.False(t, called, "flusher should not be called after SetSampleRate reduced the rate")
	})

	t.Run("WithSampleRate option sets rate on construction", func(t *testing.T) {
		t.Cleanup(stubDeviceID("test-device"))

		called := false
		svc := NewService(func(SendTelemetryPayload) { called = true }, WithSampleRate(1))

		svc.Record(ghtelemetry.Event{Type: "test"})
		svc.Flush()

		// We can't control the bucket from NewService, so we just verify
		// the service was created without error and Flush doesn't panic.
		// The actual sampling behavior is tested via direct struct manipulation above.
		_ = called
	})
}

func TestWithAdditionalCommonDimensions(t *testing.T) {
	t.Cleanup(stubDeviceID("test-device"))

	var captured SendTelemetryPayload
	svc := NewService(
		func(p SendTelemetryPayload) { captured = p },
		WithAdditionalCommonDimensions(ghtelemetry.Dimensions{
			"version": "2.45.0",
			"agent":   "none",
		}),
	)

	svc.Record(ghtelemetry.Event{Type: "test"})
	svc.Flush()

	require.Len(t, captured.Events, 1)
	assert.Equal(t, "2.45.0", captured.Events[0].Dimensions["version"])
	assert.Equal(t, "none", captured.Events[0].Dimensions["agent"])
	// Standard common dimensions should also be present
	assert.Equal(t, "test-device", captured.Events[0].Dimensions["device_id"])
	assert.NotEmpty(t, captured.Events[0].Dimensions["invocation_id"])
	assert.NotEmpty(t, captured.Events[0].Dimensions["os"])
	assert.NotEmpty(t, captured.Events[0].Dimensions["architecture"])
}

func TestServiceDisable(t *testing.T) {
	t.Run("drops recorded events from flushed payload", func(t *testing.T) {
		t.Cleanup(stubDeviceID("test-device"))

		var captured SendTelemetryPayload
		called := false
		svc := newService(func(p SendTelemetryPayload) {
			called = true
			captured = p
		}, nil)

		svc.Record(ghtelemetry.Event{Type: "test"})
		svc.Disable()
		svc.Flush()

		assert.True(t, called, "flusher should still be called so log mode can surface the absence of events")
		assert.Empty(t, captured.Events, "recorded events should be dropped after Disable()")
	})

	t.Run("drops events even with multiple recorded events", func(t *testing.T) {
		t.Cleanup(stubDeviceID("test-device"))

		var captured SendTelemetryPayload
		called := false
		svc := newService(func(p SendTelemetryPayload) {
			called = true
			captured = p
		}, nil)

		svc.Record(ghtelemetry.Event{Type: "event1"})
		svc.Record(ghtelemetry.Event{Type: "event2"})
		svc.Record(ghtelemetry.Event{Type: "event3"})
		svc.Disable()
		svc.Flush()

		assert.True(t, called, "flusher should still be called")
		assert.Empty(t, captured.Events, "recorded events should be dropped after Disable()")
	})

	t.Run("can be called before any events are recorded", func(t *testing.T) {
		t.Cleanup(stubDeviceID("test-device"))

		var captured SendTelemetryPayload
		called := false
		svc := newService(func(p SendTelemetryPayload) {
			called = true
			captured = p
		}, nil)

		svc.Disable()
		svc.Record(ghtelemetry.Event{Type: "test"})
		svc.Flush()

		assert.True(t, called, "flusher should still be called")
		assert.Empty(t, captured.Events, "events recorded after Disable() should be dropped")
	})
}

func TestNoOpService(t *testing.T) {
	svc := &NoOpService{}
	// All methods should be safe to call without panicking
	svc.Record(ghtelemetry.Event{Type: "test"})
	svc.Disable()
	svc.SetSampleRate(50)
	svc.Flush()
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
