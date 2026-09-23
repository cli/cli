package terminal

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type adapterCapture struct {
	mu     sync.Mutex
	raw    string
	events []Event
	done   chan struct{}
}

func (capture *adapterCapture) wait(t *testing.T, expected string) {
	t.Helper()
	require.Eventually(t, func() bool {
		capture.mu.Lock()
		defer capture.mu.Unlock()
		return strings.Contains(capture.raw, expected)
	}, 3*time.Second, 5*time.Millisecond, "missing fixture output %q", expected)
}

func nativeAdapterCases(t *testing.T) {
	receipt := os.Getenv("CLI_EXERCISE_TEST_RECEIPT")
	if receipt == "" {
		t.Skip("Select a genuine ready receipt for Go-driven Tuistory adapter tests.")
	}
	skillRoot, err := filepath.Abs("../../..")
	require.NoError(t, err)
	executable, err := os.Executable()
	require.NoError(t, err)
	for _, tc := range []struct {
		name    string
		action  map[string]any
		reply   string
		want    string
		resize  bool
		request map[string]any
	}{
		{name: "text", action: map[string]any{"type": "text", "text": "Mona"}, want: "Mona"},
		{name: "uppercase key", action: map[string]any{"type": "key", "key": "M"}, want: "M"},
		{name: "named key", action: map[string]any{"type": "key", "key": "down"}, want: "\x1b[B"},
		{name: "modifier chord", action: map[string]any{"type": "key", "key": []string{"ctrl", "c"}}, want: "\x03"},
		{name: "uppercase control M", action: map[string]any{"type": "key", "key": []string{"ctrl", "M"}}, want: "\r"},
		{name: "uppercase control I", action: map[string]any{"type": "key", "key": []string{"ctrl", "I"}}, want: "\t"},
		{name: "uppercase control H", action: map[string]any{"type": "key", "key": []string{"ctrl", "H"}}, want: "\x08"},
		{name: "shifted letter", action: map[string]any{"type": "key", "key": []string{"shift", "m"}}, want: "M"},
		{name: "shifted Alt letter", action: map[string]any{"type": "key", "key": []string{"shift", "alt", "m"}}, want: "\x1bM"},
		{name: "modified special key", action: map[string]any{"type": "key", "key": []string{"alt", "ctrl", "shift", "enter"}}, want: "\x1b[13;8u"},
		{name: "Alt arrow", action: map[string]any{"type": "key", "key": []string{"alt", "up"}}, want: "\x1b\x1b[A"},
		{name: "resize", action: map[string]any{"type": "resize", "columns": 90, "rows": 30}, resize: true},
		{name: "mouse", action: map[string]any{"type": "click", "x": 4, "y": 2}, want: "\x1b[<0;5;3M"},
		{name: "protocol reply", reply: "\x1b[1;1R", want: "\x1b[1;1R"},
		{name: "no controller interpreter", request: map[string]any{"op": "run_controller", "controller": map[string]any{"rules": []any{}}}},
		{name: "no semantic policy", request: map[string]any{"op": "input", "action": map[string]any{"type": "select", "label": "Apply"}}},
		{name: "no second target", request: map[string]any{"op": "open", "launch": map[string]any{}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, err := filepath.EvalSymlinks(t.TempDir())
			require.NoError(t, err)
			for _, directory := range []string{"work", "home"} {
				require.NoError(t, os.Mkdir(filepath.Join(root, directory), 0o700))
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			adapter, err := FromReceipt(ctx, skillRoot, receipt)
			require.NoError(t, err)
			capture := &adapterCapture{done: make(chan struct{})}
			go func() {
				defer close(capture.done)
				for event := range adapter.Events() {
					capture.mu.Lock()
					capture.raw += event.Raw
					capture.events = append(capture.events, event)
					capture.mu.Unlock()
				}
			}()
			closed := false
			defer func() {
				if !closed {
					cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
					defer stop()
					if err := adapter.Close(cleanup); err != nil {
						t.Logf("fixture cleanup after failure: %v", err)
					}
				}
			}()
			require.NoError(t, adapter.Start(ctx, LaunchOptions{
				Executable: executable, Args: []string{}, Cwd: filepath.Join(root, "work"),
				Env: map[string]string{"HOME": filepath.Join(root, "home"),
					"CLI_EXERCISE_TEST_FIXTURE": "bytes", "GORACE": "atexit_sleep_ms=0"},
				Columns: 80, Rows: 24,
			}))
			capture.wait(t, "READY")
			if tc.request != nil {
				require.Error(t, adapter.request(ctx, tc.request))
			} else if tc.reply != "" {
				require.NoError(t, adapter.Reply(ctx, tc.reply))
			} else {
				raw, err := json.Marshal(tc.action)
				require.NoError(t, err)
				require.NoError(t, adapter.Input(ctx, raw))
			}
			if tc.want != "" {
				capture.wait(t, "INPUT:"+hex.EncodeToString([]byte(tc.want)))
			}
			if tc.resize {
				require.Eventually(t, func() bool {
					capture.mu.Lock()
					defer capture.mu.Unlock()
					for _, event := range capture.events {
						var size struct {
							Columns int `json:"cols"`
							Rows    int `json:"rows"`
						}
						if json.Unmarshal(event.Data, &size) == nil && size.Columns == 90 && size.Rows == 30 {
							return true
						}
					}
					return false
				}, time.Second, 5*time.Millisecond, "a quiet resize must emit its new geometry before further output")
				require.NoError(t, adapter.Input(ctx, json.RawMessage(`{"type":"text","text":"?"}`)))
				capture.wait(t, "SIZE:90x30")
			}
			cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
			defer stop()
			closeErr := adapter.Close(cleanup)
			closed = true
			if tc.request != nil {
				var exit *exec.ExitError
				require.ErrorAs(t, closeErr, &exit)
				require.Equal(t, 1, exit.ExitCode())
			} else {
				require.NoError(t, closeErr)
			}
			select {
			case <-capture.done:
			case <-time.After(time.Second):
				t.Fatal("owned adapter event reader did not close")
			}
			require.NotNil(t, adapter.command.ProcessState)
			require.True(t, adapter.command.ProcessState.Exited())
			if tc.request != nil {
				capture.mu.Lock()
				raw := capture.raw
				capture.mu.Unlock()
				require.NotContains(t, raw, "INPUT:")
			}
		})
	}
}

func transportBacklogCases(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)
	for _, burst := range []int{32, 65} {
		t.Run(strconv.Itoa(burst)+" events before acknowledgement", func(t *testing.T) {
			adapter := New(executable, strconv.Itoa(burst), "transport-fixture")
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			replied := make(chan error, 1)
			failure := make(chan error, 2)
			drained := make(chan struct{})
			var frames int
			go func() {
				defer close(drained)
				for event := range adapter.Events() {
					if event.Raw == "\x1b[6n" {
						replied <- adapter.Reply(ctx, "\x1b[1;1R")
					} else if event.Kind == "data" {
						frames++
					} else if event.Err != nil {
						failure <- event.Err
					}
				}
			}()
			closed := false
			defer func() {
				if !closed {
					cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
					defer stop()
					_ = adapter.Close(cleanup)
				}
			}()
			require.NoError(t, adapter.Start(ctx, LaunchOptions{Cwd: t.TempDir(), Env: map[string]string{}}))
			select {
			case err := <-replied:
				require.NoError(t, err, "event delivery must not block the only response reader")
			case <-ctx.Done():
				t.Fatal("output backlog deadlocked a terminal query reply")
			}

			cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
			defer stop()
			closeErr := adapter.Close(cleanup)
			closed = true
			select {
			case <-drained:
			case <-cleanup.Done():
				t.Fatal("owned transport did not finish")
			}
			if burst > 64 {
				require.ErrorContains(t, closeErr, "backlog")
				select {
				case err := <-failure:
					require.ErrorContains(t, err, "backlog")
				default:
					t.Fatal("overflow must fail capture explicitly")
				}
				require.Equal(t, 64, frames, "accepted events are preserved before the failure")
			} else {
				require.NoError(t, closeErr)
				require.Equal(t, burst, frames)
				require.Empty(t, failure)
			}
			require.True(t, adapter.command.ProcessState.Exited())
		})
	}
}

func processGroupProtocolCases(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)
	for _, tc := range []struct {
		name string
		mode int
	}{
		{name: "missing group", mode: -1},
		{name: "init group", mode: -2},
		{name: "negative group", mode: -3},
		{name: "adapter group is not the terminal group", mode: -4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			adapter := New(executable, strconv.Itoa(tc.mode), "transport-fixture")
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			startErr := adapter.Start(ctx, LaunchOptions{Cwd: t.TempDir(), Env: map[string]string{}})
			cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
			defer stop()
			closeErr := adapter.Close(cleanup)
			require.ErrorContains(t, closeErr, "invalid owned process group", "start result: %v", startErr)
			require.Zero(t, adapter.processGroup.Load(), "invalid identities must never become cleanup targets")
			require.NotNil(t, adapter.command.ProcessState)
		})
	}
}

func TestRun(t *testing.T) {
	t.Run("transport backpressure", transportBacklogCases)
	t.Run("owned process group metadata", processGroupProtocolCases)
	t.Run("native mechanical adapter", nativeAdapterCases)
	t.Run("native process cleanup", nativeCleanupCases)
	t.Run("native output buffering", nativeOutputBudgetCases)
}
