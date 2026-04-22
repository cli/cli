// Package telemetry provides best-effort usage telemetry for gh commands.
package telemetry

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh/ghtelemetry"
	"github.com/cli/cli/v2/pkg/jsoncolor"
	"github.com/google/uuid"
	"github.com/mgutz/ansi"
)

const deviceIDFileName = "device-id"

// stateDirFunc returns the state directory path. Can be replaced in tests.
var stateDirFunc = config.StateDir

// deviceIDFunc returns a per-user device identifier stored in the state directory.
// It generates and persists a UUID on first call. Can be replaced in tests.
var deviceIDFunc = getOrCreateDeviceID

func getOrCreateDeviceID() (string, error) {
	stateDir := stateDirFunc()
	idPath := filepath.Join(stateDir, deviceIDFileName)

	data, err := os.ReadFile(idPath)
	if err == nil {
		return strings.TrimSpace(string(data)), nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	id := uuid.New().String()
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return "", err
	}

	// Write the ID to a temp file in the same directory, then hard-link it
	// to the target path. os.Link fails atomically if the target already
	// exists, so exactly one concurrent caller wins. Losers read the
	// winner's ID. The temp file is always cleaned up.
	tmpFile, err := os.CreateTemp(stateDir, deviceIDFileName+".tmp.*")
	if err != nil {
		return "", err
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.WriteString(id); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return "", err
	}
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return "", err
	}

	linkErr := os.Link(tmpPath, idPath)
	os.Remove(tmpPath)

	if linkErr != nil {
		// Another caller won — read their ID.
		data, readErr := os.ReadFile(idPath)
		if readErr != nil {
			return "", linkErr
		}
		return strings.TrimSpace(string(data)), nil
	}

	return id, nil
}

var falseyValues = []string{"", "0", "false", "no", "disabled", "off"}

// lookupEnvFunc wraps os.LookupEnv. Can be replaced in tests.
var lookupEnvFunc = os.LookupEnv

type TelemetryState string

const (
	Enabled  TelemetryState = "enabled"
	Disabled TelemetryState = "disabled"
	Logged   TelemetryState = "log"
)

// ParseTelemetryState determines the telemetry state based on environment variables and configuration values.
// The GH_TELEMETRY environment variable takes precedence, followed by DO_NOT_TRACK, then the configuration value.
// Recognized values for GH_TELEMETRY and config are "enabled", "disabled", "log", or any falsey value (e.g. "0", "false", "no") to disable telemetry.
func ParseTelemetryState(configValue string) TelemetryState {
	// GH_TELEMETRY env var takes highest precedence
	if envVal, ok := lookupEnvFunc("GH_TELEMETRY"); ok {
		envVal = strings.TrimSpace(strings.ToLower(envVal))

		// If falsey, telemetry is disabled.
		if slices.Contains(falseyValues, envVal) {
			return Disabled
		}

		// If logged, telemetry is logged instead of sent.
		if envVal == "log" {
			return Logged
		}

		// Any other value (including "enabled") is treated as enabled.
		return Enabled
	}

	// DO_NOT_TRACK takes precedence over config
	if envVal, ok := lookupEnvFunc("DO_NOT_TRACK"); ok {
		envVal = strings.TrimSpace(strings.ToLower(envVal))
		if envVal == "1" || envVal == "true" {
			return Disabled
		}
	}

	// Then check the config values with the same rules.
	configValue = strings.TrimSpace(strings.ToLower(configValue))

	if slices.Contains(falseyValues, configValue) {
		return Disabled
	}

	if configValue == "log" {
		return Logged
	}

	return Enabled
}

type telemetryServiceOpts struct {
	additionalDimensions ghtelemetry.Dimensions
	sampleRate           int
}

type telemetryServiceOption func(*telemetryServiceOpts)

// WithAdditionalCommonDimensions allows setting additional common dimensions that will be included with every telemetry event recorded by the service.
func WithAdditionalCommonDimensions(dimensions ghtelemetry.Dimensions) telemetryServiceOption {
	return func(s *telemetryServiceOpts) {
		maps.Copy(s.additionalDimensions, dimensions)
	}
}

// WithSampleRate allows setting a sample rate (0-100) for telemetry events. Events recorded with the Unsampled option will be sent regardless of the sample rate.
// Sampling is based on invocation ID, so an entire invocation will be included or excluded as a whole. This ensures that related events are not split between sampled and unsampled,
// which could lead to incomplete data and incorrect assumptions.
func WithSampleRate(rate int) telemetryServiceOption {
	return func(s *telemetryServiceOpts) {
		s.sampleRate = rate
	}
}

// LogFlusher returns a flush function that writes telemetry payloads to the provided log writer. This is used for the "log" telemetry mode, which is intended for debugging and development.
// When there are no events to report (for example the command opted out of telemetry, the user is on GHES, or no events were recorded), a "Telemetry payload: none" marker is written so that the absence of events is observable.
var LogFlusher = func(log io.Writer, colorEnabled bool) func(payload SendTelemetryPayload) {
	return func(payload SendTelemetryPayload) {
		header := "Telemetry payload:"
		if colorEnabled {
			header = ansi.Color(header, "cyan+b")
		}

		if len(payload.Events) == 0 {
			fmt.Fprintf(log, "%s none\n", header)
			return
		}

		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			return
		}

		fmt.Fprintf(log, "%s\n", header)

		if colorEnabled {
			_ = jsoncolor.Write(log, bytes.NewReader(payloadBytes), "  ")
		} else {
			var indented bytes.Buffer
			_ = json.Indent(&indented, payloadBytes, "", "  ")
			fmt.Fprintln(log, indented.String())
		}
	}
}

// GitHubFlusher returns a flush function that sends telemetry payloads to a child `gh send-telemetry` process. This is used for the "enabled" telemetry mode.
// Empty payloads are dropped without spawning a subprocess.
var GitHubFlusher = func(executable string) func(payload SendTelemetryPayload) {
	return func(payload SendTelemetryPayload) {
		if len(payload.Events) == 0 {
			return
		}
		SpawnSendTelemetry(executable, payload)
	}
}

// NewService creates a new telemetry service with the provided flush function and options.
func NewService(flusher func(SendTelemetryPayload), opts ...telemetryServiceOption) ghtelemetry.Service {
	telemetryServiceOpts := telemetryServiceOpts{
		additionalDimensions: make(ghtelemetry.Dimensions),
	}
	for _, opt := range opts {
		opt(&telemetryServiceOpts)
	}

	deviceID, err := deviceIDFunc()
	if err != nil {
		deviceID = "<unknown>"
	}

	invocationID := uuid.NewString()

	var commonDimensions = ghtelemetry.Dimensions{
		"device_id":     deviceID,
		"invocation_id": invocationID,
		"os":            runtime.GOOS,
		"architecture":  runtime.GOARCH,
	}
	maps.Copy(commonDimensions, telemetryServiceOpts.additionalDimensions)

	hash := uuid.NewSHA1(uuid.Nil, []byte(invocationID))
	sampleBucket := byte(binary.BigEndian.Uint32(hash[:4]) % 100)

	s := &service{
		flush:            flusher,
		commonDimensions: commonDimensions,
		sampleRate:       telemetryServiceOpts.sampleRate,
		sampleBucket:     sampleBucket,
	}

	return s
}

type recordedEvent struct {
	event      ghtelemetry.Event
	recordedAt time.Time
}

type service struct {
	mu               sync.RWMutex
	flush            func(payload SendTelemetryPayload)
	previouslyCalled bool

	commonDimensions ghtelemetry.Dimensions
	sampleRate       int
	sampleBucket     byte

	events []recordedEvent

	disabled bool
}

func (s *service) Disable() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.disabled = true
}

func (s *service) Record(event ghtelemetry.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.events = append(s.events, recordedEvent{event: event, recordedAt: time.Now()})
}

func (s *service) SetSampleRate(rate int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sampleRate = rate
}

func (s *service) Flush() {
	// This shouldn't really be required since flush should only be called once, but just in case...
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.previouslyCalled {
		return
	}
	s.previouslyCalled = true

	if s.sampleRate > 0 && s.sampleRate < 100 && int(s.sampleBucket) >= s.sampleRate {
		return
	}

	// When the service has been disabled mid-invocation (e.g. an enterprise host
	// was contacted), discard any recorded events. We still call the flusher
	// with an empty payload so that the log-mode flusher can surface the
	// absence of telemetry rather than leaving the user staring at silence.
	events := s.events
	if s.disabled {
		events = nil
	}

	payload := SendTelemetryPayload{
		Events: make([]PayloadEvent, len(events)),
	}

	for i, recorded := range events {
		dimensions := map[string]string{
			"timestamp": recorded.recordedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
		}
		maps.Copy(dimensions, s.commonDimensions)
		maps.Copy(dimensions, recorded.event.Dimensions)

		payload.Events[i] = PayloadEvent{
			Type:       recorded.event.Type,
			Dimensions: dimensions,
			Measures:   recorded.event.Measures,
		}
	}

	s.flush(payload)
}

// maxPayloadSize is a safety limit for the telemetry payload written to the
// child process stdin pipe. This bounds the data transferred to a reasonable
// size and avoids blocking on pipe buffer capacity (typically 16-64 KB).
const maxPayloadSize = 16 * 1024

// PayloadEvent represents a single telemetry event in the wire format.
type PayloadEvent struct {
	Type       string            `json:"type"`
	Dimensions map[string]string `json:"dimensions,omitempty"`
	Measures   map[string]int64  `json:"measures,omitempty"`
}

type SendTelemetryPayload struct {
	Events []PayloadEvent `json:"events"`
}

// SpawnSendTelemetry spawns a detached subprocess to send telemetry.
// The payload is written to the child's stdin via a pipe so that it is not
// visible to other users through process argument inspection (e.g. ps aux).
// The parent writes the full payload and closes the pipe before returning,
// so no long-lived pipe is needed and the parent can exit immediately.
//
// Note: the payload is bounded by maxPayloadSize (16 KB). On macOS the
// default pipe buffer is also 16 KB, so in theory a write could block
// briefly if the child hasn't started reading yet. In practice the child
// is already running after cmd.Start(), so this is unlikely.
//
// All errors are silently ignored since telemetry is best-effort.
func SpawnSendTelemetry(executable string, payload SendTelemetryPayload) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return
	}

	if len(payloadBytes) > maxPayloadSize {
		return
	}

	// Resolve the executable to an absolute path before changing the child's
	// working directory. Without this, a relative path (e.g. from GH_PATH) would
	// be resolved against cmd.Dir at Start time and fail to spawn.
	if abs, err := filepath.Abs(executable); err == nil {
		executable = abs
	}

	cmd := exec.Command(executable, "send-telemetry")

	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard

	// Set the working directory to a stable directory elsewhere so that the subprocess doesn't
	// hold a reference to the parent's current working directory, avoiding any weirdness around
	// deleting the parent process's current working directory while the child is still running.
	// Only do this when we have an absolute executable path so that the child can still be found.
	if filepath.IsAbs(executable) {
		cmd.Dir = os.TempDir()
	}

	// Configure the child process to be detached from the parent so that it can continue running
	// after the parent exits, and so that it doesn't receive any signals sent to the parent.
	cmd.SysProcAttr = detachAttrs()

	// Get the write end of the stdin pipe before starting.
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return
	}

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return
	}

	// Write the payload synchronously into the kernel pipe buffer, then close
	// the pipe to signal EOF. The child reads the complete payload from stdin.
	// io.Copy loops until all bytes are written, avoiding any risk of a short write.
	_, _ = io.Copy(stdin, bytes.NewReader(payloadBytes))
	_ = stdin.Close()

	// Release resources associated with the child process since we will never Wait for it.
	_ = cmd.Process.Release()
}

type NoOpService struct{}

func (s *NoOpService) Record(event ghtelemetry.Event) {}

func (s *NoOpService) Disable() {}

func (s *NoOpService) SetSampleRate(rate int) {}

func (s *NoOpService) Flush() {}
