package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cli/cli/v2/cli-exercise/internal/terminal"
	"github.com/stretchr/testify/require"
)

type fakeTerminal struct {
	mu             sync.Mutex
	events         chan terminal.Event
	inputs         []document
	launch         terminal.LaunchOptions
	text           string
	closed         bool
	onStart        func() error
	onClose        func(context.Context) error
	onInput        func(document, *fakeTerminal) error
	onInputContext func(context.Context, document) error
}

func (fake *fakeTerminal) Events() <-chan terminal.Event { return fake.events }
func (fake *fakeTerminal) Start(_ context.Context, launch terminal.LaunchOptions) error {
	fake.launch = launch
	if fake.onStart != nil {
		if err := fake.onStart(); err != nil {
			return err
		}
	}
	fake.emit("Ready", "Ready")
	return nil
}
func (fake *fakeTerminal) emit(text, raw string) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.closed {
		return
	}
	fake.text = text
	var lines []any
	for line := range strings.SplitSeq(text, "\n") {
		lines = append(lines, document{"spans": []any{document{"text": line, "width": len(line), "flags": 0}}})
	}
	data, _ := json.Marshal(document{"cols": fake.launch.Columns, "rows": fake.launch.Rows,
		"lines": lines, "cursor": []int{0, 0}, "cursorVisible": true, "cursorStyle": "block"})
	fake.events <- terminal.Event{Kind: "data", Data: data, Raw: raw}
}
func (fake *fakeTerminal) Input(ctx context.Context, raw json.RawMessage) error {
	var action document
	if err := json.Unmarshal(raw, &action); err != nil {
		return err
	}
	fake.mu.Lock()
	fake.inputs = append(fake.inputs, action)
	fake.mu.Unlock()
	if fake.onInputContext != nil {
		return fake.onInputContext(ctx, action)
	}
	if fake.onInput != nil {
		return fake.onInput(action, fake)
	}
	return nil
}
func (fake *fakeTerminal) Reply(context.Context, string) error { return nil }
func (fake *fakeTerminal) Close(ctx context.Context) error {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	var err error
	if !fake.closed {
		if fake.onClose != nil {
			err = fake.onClose(ctx)
		}
		fake.closed = true
		close(fake.events)
	}
	return err
}
func (fake *fakeTerminal) exit(code, signal int) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if !fake.closed {
		fake.events <- terminal.Event{Kind: "exit", ExitCode: &code, Signal: &signal}
	}
}

type fixture struct {
	root, source, workspace string
	contract                document
	fake                    *fakeTerminal
	runtime                 *Runtime
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	root, err := filepath.EvalSymlinks(root)
	require.NoError(t, err)
	repository := filepath.Join(root, "repository")
	require.NoError(t, os.MkdirAll(filepath.Join(repository, ".git"), 0o700))
	t.Chdir(repository)
	executable := filepath.Join(root, "fixture-program")
	data := []byte("Synthetic executable identity.\n")
	require.NoError(t, os.WriteFile(executable, data, 0o700))
	hash := sha256.Sum256(data)
	fixture := &fixture{root: root, source: filepath.Join(root, "case.json"), workspace: filepath.Join(root, "run")}
	fixture.contract = document{
		"schemaVersion": 1, "caseId": "fixture", "mode": "exact", "goal": "Exercise only the synthetic fixture.",
		"source":        document{"kind": "explicit", "text": "Preserve the supplied synthetic case."},
		"workspace":     fixture.workspace,
		"command":       document{"executable": executable, "args": []any{}, "sha256": hex.EncodeToString(hash[:]), "version": "fixture 1", "cwd": "work"},
		"environment":   document{"values": document{}, "pass": []any{}},
		"authorization": document{"status": "approved", "basis": "The caller approved this local fixture.", "effects": []any{}},
		"terminal": document{"columns": 80, "rows": 24, "fontSize": 18,
			"fps": 30, "background": "#0d1117", "foreground": "#e6edf3"},
		"limits": document{"maxActions": 100, "maxDurationSeconds": 10, "idleTimeoutSeconds": 5},
		"steps":  []any{}, "constraints": document{"deny": []any{}}, "expectations": []any{},
		"output": document{"formats": []any{"gif"}, "timing": "realtime"},
	}
	fixture.fake = &fakeTerminal{events: make(chan terminal.Event, 200)}
	t.Cleanup(func() {
		if fixture.runtime != nil {
			_, err := fixture.runtime.Finish(context.Background(), "test_cleanup", true)
			require.NoError(t, err)
		}
	})
	return fixture
}

func (fixture *fixture) prepare(t *testing.T, passed map[string]string) (*Runtime, error) {
	t.Helper()
	raw, err := json.Marshal(fixture.contract)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(fixture.source, raw, 0o600))
	runtime, err := New(Options{ContractPath: fixture.source, PassedEnvironment: passed, Terminal: fixture.fake})
	if err == nil {
		fixture.runtime = runtime
	}
	return runtime, err
}

func (fixture *fixture) start(t *testing.T, passed map[string]string) {
	t.Helper()
	runtime, err := fixture.prepare(t, passed)
	require.NoError(t, err)
	require.NoError(t, runtime.Start(context.Background()))
	fixture.waitText(t, "Ready")
}

func (fixture *fixture) request(t *testing.T, value document) document {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	response, err := fixture.runtime.Handle(context.Background(), raw)
	require.NoError(t, err)
	var result document
	require.NoError(t, json.Unmarshal(response, &result))
	return result
}

func (fixture *fixture) waitText(t *testing.T, text string) document {
	t.Helper()
	response := fixture.request(t, document{"op": "wait", "until": document{"screenContains": text}, "timeoutMs": 1000})
	require.Equal(t, true, response["ok"], response)
	return response
}

func (fixture *fixture) act(t *testing.T, action document) document {
	t.Helper()
	observed := fixture.request(t, document{"op": "observe"})
	return fixture.request(t, document{"op": "act", "revision": observed["revision"], "reason": "Execute the supplied case input.", "action": action})
}

func (fixture *fixture) outcome(t *testing.T) document {
	t.Helper()
	select {
	case <-fixture.runtime.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("owned runtime did not finish")
	}
	raw, ok := fixture.runtime.Result()
	require.True(t, ok)
	var value document
	require.NoError(t, json.Unmarshal(raw, &value))
	return value
}

func errorCode(response document) string {
	error, _ := object(response["error"])
	return str(error["code"])
}

func TestContractAndActionValidation(t *testing.T) {
	t.Run("native key support", keyValidationCases)
	for _, tc := range []struct {
		name   string
		change func(*fixture)
		passed map[string]string
		code   string
	}{
		{name: "valid immutable source"},
		{name: "unapproved", change: func(f *fixture) { f.contract["authorization"].(document)["status"] = "pending" }, code: "invalid_contract"},
		{name: "changed executable", change: func(f *fixture) { f.contract["command"].(document)["sha256"] = strings.Repeat("0", 64) }, code: "executable_changed"},
		{name: "cwd traversal", change: func(f *fixture) { f.contract["command"].(document)["cwd"] = "../outside" }, code: "invalid_contract"},
		{name: "relative shared state rejected", change: func(f *fixture) { f.contract["stateDirectory"] = "state" }, code: "invalid_contract"},
		{name: "shared state cannot contain capture", change: func(f *fixture) { f.contract["stateDirectory"] = f.root }, code: "unsafe_workspace"},
		{name: "shared state cannot overlap capture descendants", change: func(f *fixture) { f.contract["stateDirectory"] = filepath.Join(f.workspace, "state") }, code: "unsafe_workspace"},
		{name: "missing environment", change: func(f *fixture) {
			f.contract["environment"].(document)["pass"] = []any{document{"name": "FIXTURE_SECRET", "secret": true}}
		}, code: "missing_environment"},
		{name: "secret in original source", passed: map[string]string{"FIXTURE_SECRET": "synthetic-private"}, change: func(f *fixture) {
			f.contract["environment"].(document)["pass"] = []any{document{"name": "FIXTURE_SECRET", "secret": true}}
			f.contract["source"].(document)["text"] = "synthetic-private"
		}, code: "private_contract"},
		{name: "secret split across steps", passed: map[string]string{"FIXTURE_SECRET": "synthetic-private"}, change: func(f *fixture) {
			f.contract["environment"].(document)["pass"] = []any{document{"name": "FIXTURE_SECRET", "secret": true}}
			f.contract["steps"] = []any{document{"type": "text", "text": "synthetic-"}, document{"type": "text", "text": "private"}}
		}, code: "private_contract"},
		{name: "reserved environment", change: func(f *fixture) { f.contract["environment"].(document)["values"] = document{"HOME": "/elsewhere"} }, code: "invalid_contract"},
		{name: "unsupported cadence", change: func(f *fixture) { f.contract["terminal"].(document)["fps"] = 120 }, code: "invalid_contract"},
		{name: "unspecified timing defaults to condensed", change: func(f *fixture) { delete(f.contract["output"].(document), "timing") }},
		{name: "condensed needs no separate approval", change: func(f *fixture) { f.contract["output"].(document)["timing"] = "condensed" }},
		{name: "explicit realtime permits unannotated output", change: func(f *fixture) { f.contract["output"].(document)["captions"] = false }},
		{name: "unannotated output preserves real timing", change: func(f *fixture) {
			f.contract["output"].(document)["timing"] = "condensed"
			f.contract["output"].(document)["captions"] = false
		}, code: "invalid_contract"},
		{name: "unannotated output requires explicit realtime", change: func(f *fixture) {
			delete(f.contract["output"].(document), "timing")
			f.contract["output"].(document)["captions"] = false
		}, code: "invalid_contract"},
		{name: "blank timing is not an omitted value", change: func(f *fixture) { f.contract["output"].(document)["timing"] = "" }, code: "invalid_contract"},
		{name: "null timing is not an omitted value", change: func(f *fixture) { f.contract["output"].(document)["timing"] = nil }, code: "invalid_contract"},
		{name: "unknown timing is rejected", change: func(f *fixture) { f.contract["output"].(document)["timing"] = "fast" }, code: "invalid_contract"},
		{name: "raw control input", change: func(f *fixture) { f.contract["steps"] = []any{document{"type": "text", "text": "\x1b"}} }, code: "invalid_action"},
		{name: "key sequence is not a chord", change: func(f *fixture) {
			f.contract["steps"] = []any{document{"type": "key", "key": []any{"down", "down"}}}
		}, code: "invalid_action"},
		{name: "condition code is not accepted", change: func(f *fixture) {
			f.contract["constraints"] = document{"deny": []any{document{"action": document{"type": "key"}, "when": document{"regex": ".*"}}}}
		}, code: "invalid_condition"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newFixture(t)
			if tc.change != nil {
				tc.change(fixture)
			}
			runtime, err := fixture.prepare(t, tc.passed)
			if tc.code != "" {
				require.Error(t, err)
				require.Equal(t, tc.code, safeFault(err).Code, err)
				return
			}
			require.NoError(t, err)
			source, err := os.ReadFile(fixture.source)
			require.NoError(t, err)
			saved, err := os.ReadFile(filepath.Join(runtime.Workspace(), "contract.json"))
			require.NoError(t, err)
			require.Equal(t, source, saved)
			require.Empty(t, fixture.fake.inputs)
		})
	}
}

func TestRuntime(t *testing.T) {
	t.Run("execution timestamps", runtimeTimestampCases)
	t.Run("shared case state survives separate recorded invocations", func(t *testing.T) {
		first := newFixture(t)
		shared := filepath.Join(first.root, "case-state")
		first.contract["stateDirectory"] = shared
		first.start(t, nil)
		require.Equal(t, filepath.Join(shared, "work"), first.fake.launch.Cwd)
		marker := filepath.Join(first.fake.launch.Env["XDG_CONFIG_HOME"], "setup.txt")
		require.NoError(t, os.WriteFile(marker, []byte("created during setup"), 0o600))
		first.fake.exit(0, 0)
		firstResult := first.outcome(t)
		require.Equal(t, "complete", firstResult["captureStatus"])
		second := newFixture(t)
		second.contract["stateDirectory"] = shared
		second.start(t, nil)
		require.Equal(t, first.fake.launch.Env["HOME"], second.fake.launch.Env["HOME"])
		value, err := os.ReadFile(filepath.Join(second.fake.launch.Env["XDG_CONFIG_HOME"], "setup.txt"))
		require.NoError(t, err)
		require.Equal(t, "created during setup", string(value))
		require.NotEqual(t, first.runtime.Workspace(), second.runtime.Workspace())
		firstContract, err := os.ReadFile(filepath.Join(first.runtime.Workspace(), "contract.json"))
		require.NoError(t, err)
		second.fake.exit(0, 0)
		secondResult := second.outcome(t)
		require.Equal(t, "complete", secondResult["captureStatus"])
		require.False(t, runtimeTimestamp(t, secondResult, "startedAt").Before(runtimeTimestamp(t, firstResult, "finishedAt")))
		preserved, err := os.ReadFile(filepath.Join(first.runtime.Workspace(), "contract.json"))
		require.NoError(t, err)
		require.Equal(t, firstContract, preserved)
	})
	t.Run("typed observations retain the original terminal JSON", func(t *testing.T) {
		fixture := newFixture(t)
		fixture.start(t, nil)
		original := json.RawMessage(`{"cols":80,"rows":24,"cursor":[1,0],"cursorVisible":true,"cursorStyle":"block",
			"offset":0,"totalLines":24,"futureMetadata":{"value":"retained"},
			"lines":[{"spans":[{"text":"Typed viewport","width":14,"flags":1,"fg":null,"futureStyle":7}]}]}`)
		fixture.fake.events <- terminal.Event{Kind: "data", Data: original, Raw: "Typed viewport"}
		fixture.waitText(t, "Typed viewport")
		fixture.fake.exit(0, 0)
		require.Equal(t, "complete", fixture.outcome(t)["captureStatus"])
		journal, err := os.ReadFile(filepath.Join(fixture.workspace, "capture/states.jsonl"))
		require.NoError(t, err)
		var last struct {
			Data json.RawMessage `json:"data"`
		}
		lines := strings.Split(strings.TrimSpace(string(journal)), "\n")
		require.NoError(t, json.Unmarshal([]byte(lines[len(lines)-1]), &last))
		require.JSONEq(t, string(original), string(last.Data), "typed access must not discard unknown fields or rewrite nulls")
	})
	t.Run("shared assertions", assertionCases)
	t.Run("equivalent denial keys", denialKeyCases)
	t.Run("controller action deadlines", controllerDeadlineCases)
	t.Run("duration cancels pending input", durationCancellationCase)
	for _, name := range []string{
		"empty exact gate", "ordering and values", "stale observation", "unsupported selector",
		"authorized confirmation", "generated primitive denial", "partial text stops",
		"raw text and chord", "asynchronous controller", "controller code rejected", "controller budgets",
		"normal failure", "expected nonzero", "no assertions", "unknown and manual assertions",
		"transport failure blocks capture",
		"interrupted", "source changed", "split secret output", "invisible secret output",
		"timing window", "isolated environment", "late pending output",
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newFixture(t)
			step := document{"type": "key", "key": "x"}
			fixture.contract["steps"] = []any{step}
			passed := map[string]string{}
			switch name {
			case "empty exact gate", "normal failure", "expected nonzero", "no assertions", "unknown and manual assertions", "split secret output", "invisible secret output", "late pending output", "transport failure blocks capture":
				fixture.contract["steps"] = []any{}
			}
			switch name {
			case "split secret output", "invisible secret output", "late pending output":
				fixture.contract["environment"].(document)["pass"] = []any{document{"name": "FIXTURE_SECRET", "secret": true}}
				passed["FIXTURE_SECRET"] = "ghp_synthetic_private"
			case "normal failure":
				fixture.contract["expectations"] = []any{document{"id": "exit", "type": "exit_code", "value": 0}}
			case "expected nonzero":
				fixture.contract["expectations"] = []any{document{"id": "exit", "type": "exit_code", "value": 2}}
			case "unknown and manual assertions":
				fixture.contract["expectations"] = []any{document{"id": "one", "type": "manual"}, document{"id": "two", "type": "file_exists"}}
			case "authorized confirmation":
				fixture.contract["steps"] = []any{document{"type": "select", "label": "Apply"}}
			case "generated primitive denial":
				fixture.contract["steps"] = []any{document{"type": "select", "label": "Blue"}}
				fixture.contract["constraints"] = document{"deny": []any{document{
					"action": document{"type": "key", "key": "enter"}, "when": document{"selected": "Blue"},
					"reason": "This fixture does not authorize confirmation.",
				}}}
				fixture.fake.onInput = func(action document, fake *fakeTerminal) error {
					if action["key"] == "down" {
						fake.emit("Choose\n  Red\n> Blue", "")
					}
					return nil
				}
			case "partial text stops":
				fixture.contract["steps"] = []any{document{"type": "text", "text": "abc"}}
				fixture.contract["constraints"] = document{"deny": []any{document{"action": document{"type": "text", "text": "c"}, "reason": "No final character."}}}
			case "raw text and chord":
				fixture.contract["steps"] = []any{document{"type": "text", "text": "Mona"}, document{"type": "key", "key": []any{"ctrl", "c"}}}
			case "controller budgets", "unsupported selector":
				fixture.contract["mode"] = "explore"
			case "asynchronous controller":
				fixture.contract["steps"] = []any{document{"type": "key", "key": "x"}, document{"type": "key", "key": "y"}}
				fixture.fake.onInput = func(action document, fake *fakeTerminal) error {
					next := "Next"
					if action["key"] == "y" {
						next = "Uncovered"
					}
					time.AfterFunc(15*time.Millisecond, func() { fake.emit(next, next) })
					return nil
				}
			case "timing window":
				fixture.contract["steps"] = []any{document{"type": "key", "key": "x", "timing": document{"maxDelayBeforeMs": 1}}}
			case "isolated environment":
				fixture.contract["environment"] = document{"values": document{"APP_SETTING": "reviewed"}, "pass": []any{document{"name": "APP_VALUE", "secret": false}}}
				passed["APP_VALUE"], passed["UNAPPROVED"] = "allowed", "not forwarded"
			}
			fixture.start(t, passed)
			switch name {
			case "empty exact gate", "ordering and values":
				wrong := document{"type": "key", "key": "y"}
				require.Equal(t, "exact_mismatch", errorCode(fixture.act(t, wrong)))
				require.Empty(t, fixture.fake.inputs)
			case "stale observation":
				old := fixture.request(t, document{"op": "observe"})
				fixture.fake.emit("Changed", "Changed")
				fixture.waitText(t, "Changed")
				response := fixture.request(t, document{"op": "act", "revision": old["revision"], "reason": "Use stale state.", "action": step})
				require.Equal(t, "stale_observation", errorCode(response))
			case "unsupported selector":
				require.Equal(t, "unsupported_selector", errorCode(fixture.act(t, document{"type": "select", "label": "Next"})))
				require.Empty(t, fixture.fake.inputs)
			case "authorized confirmation":
				fixture.fake.emit("Choose\n> Apply\n  Leave", "")
				fixture.waitText(t, "Apply")
				require.Equal(t, true, fixture.act(t, document{"type": "select", "label": "Apply"})["ok"])
				require.Equal(t, "enter", fixture.fake.inputs[0]["key"])
			case "generated primitive denial":
				fixture.fake.emit("Choose\n> Red\n  Blue", "")
				fixture.waitText(t, "Choose")
				require.Equal(t, "action_denied", errorCode(fixture.act(t, document{"type": "select", "label": "Blue"})))
				require.Equal(t, "blocked", fixture.outcome(t)["caseStatus"])
				require.Len(t, fixture.fake.inputs, 1)
			case "partial text stops":
				require.Equal(t, "action_denied", errorCode(fixture.act(t, document{"type": "text", "text": "abc"})))
				require.Equal(t, "partial_action", fixture.outcome(t)["stopReason"])
				require.Len(t, fixture.fake.inputs, 2)
			case "raw text and chord":
				require.Equal(t, true, fixture.act(t, document{"type": "text", "text": "Mona"})["ok"])
				require.Equal(t, true, fixture.act(t, document{"type": "key", "key": []any{"ctrl", "c"}})["ok"])
				require.Len(t, fixture.fake.inputs, 5)
			case "asynchronous controller", "controller budgets", "controller code rejected":
				observed := fixture.request(t, document{"op": "observe"})
				controller := document{"waitForMatchMs": 50, "rules": []any{
					document{"id": "first", "when": document{"screenContains": "Ready"}, "action": step, "reason": "First supplied key."},
					document{"id": "second", "when": document{"screenContains": "Next"}, "action": document{"type": "key", "key": "y"}, "reason": "Second supplied key."},
				}}
				if name == "controller budgets" {
					controller = document{"maxActions": 3, "rules": []any{document{"id": "repeat", "when": document{}, "action": step, "reason": "Bounded input.", "maxMatches": 10}}}
				} else if name == "controller code rejected" {
					controller["code"] = "not executable"
				}
				response := fixture.request(t, document{"op": "run_controller", "revision": observed["revision"], "reason": "Run the supplied rules.", "controller": controller})
				if name == "controller code rejected" {
					require.Equal(t, "invalid_controller", errorCode(response))
				} else if name == "controller budgets" {
					require.Equal(t, "controller_limit", response["reason"])
					require.Len(t, fixture.fake.inputs, 3)
				} else {
					require.Equal(t, float64(2), response["actions"], response)
					require.Equal(t, "unhandled_state", response["reason"])
					require.Len(t, fixture.fake.inputs, 2)
				}
			case "normal failure", "expected nonzero", "no assertions", "unknown and manual assertions":
				code := 0
				if name == "normal failure" || name == "expected nonzero" {
					code = 2
				}
				fixture.fake.exit(code, 0)
				result := fixture.outcome(t)
				require.Equal(t, "complete", result["captureStatus"])
				want := map[string]string{"normal failure": "failed", "expected nonzero": "passed", "no assertions": "observed", "unknown and manual assertions": "blocked"}[name]
				require.Equal(t, want, result["caseStatus"])
			case "interrupted":
				_, err := fixture.runtime.Finish(context.Background(), "shutdown", true)
				require.NoError(t, err)
				require.Equal(t, "blocked", fixture.outcome(t)["caseStatus"])
			case "transport failure blocks capture":
				fixture.fake.events <- terminal.Event{Kind: "error", Err: problem("capture_limit", "The terminal event backlog is full.")}
				result := fixture.outcome(t)
				require.Equal(t, "failed", result["captureStatus"])
				require.Equal(t, "blocked", result["caseStatus"])
			case "source changed":
				require.NoError(t, os.Chmod(filepath.Join(fixture.workspace, "contract.json"), 0o600))
				require.NoError(t, os.WriteFile(filepath.Join(fixture.workspace, "contract.json"), []byte("{}"), 0o600))
				require.Equal(t, "contract_modified", errorCode(fixture.act(t, step)))
				require.Equal(t, "failed", fixture.outcome(t)["captureStatus"])
			case "split secret output", "invisible secret output":
				prefix, suffix := "ghp_synthetic_", "private"
				if name == "invisible secret output" {
					prefix, suffix = "\x1b]0;"+prefix, suffix+"\x07"
				}
				fixture.fake.emit("Ready", prefix)
				fixture.fake.emit("Ready", suffix)
				require.Equal(t, "failed", fixture.outcome(t)["captureStatus"])
				for _, file := range []string{"states.jsonl", "events.jsonl", "raw.jsonl"} {
					data, err := os.ReadFile(filepath.Join(fixture.workspace, "capture", file))
					require.NoError(t, err)
					require.NotContains(t, string(data), "ghp_synthetic_")
				}
			case "late pending output":
				fixture.fake.emit("g", "g")
				fixture.fake.emit("go", "o")
				fixture.fake.exit(0, 0)
				require.Equal(t, "complete", fixture.outcome(t)["captureStatus"])
			case "timing window":
				time.Sleep(5 * time.Millisecond)
				require.Equal(t, "timing_mismatch", errorCode(fixture.act(t, document{"type": "key", "key": "x", "timing": document{"maxDelayBeforeMs": 1}})))
				require.Empty(t, fixture.fake.inputs)
			case "isolated environment":
				require.Equal(t, filepath.Join(fixture.workspace, "home"), fixture.fake.launch.Env["HOME"])
				require.Equal(t, "allowed", fixture.fake.launch.Env["APP_VALUE"])
				require.NotContains(t, fixture.fake.launch.Env, "UNAPPROVED")
			}
		})
	}
}

func runtimeTimestamp(t *testing.T, result document, field string) time.Time {
	t.Helper()
	value, ok := result[field].(string)
	require.True(t, ok, "%s must be a timestamp string", field)
	stamp, err := time.Parse(time.RFC3339Nano, value)
	require.NoError(t, err)
	require.Equal(t, stamp.UTC().Format(time.RFC3339Nano), value)
	return stamp
}

func runtimeTimestampCases(t *testing.T) {
	t.Run("execution and owned cleanup are covered without changing capture timing", func(t *testing.T) {
		fixture := newFixture(t)
		step := document{"type": "key", "key": "x"}
		fixture.contract["steps"] = []any{step}
		fixture.contract["expectations"] = []any{document{"id": "exit", "type": "exit_code", "value": 0}}
		var launchedAt, cleanupBegan, cleanupEnded time.Time
		cleanupStarted, cleanupRelease := make(chan struct{}), make(chan struct{})
		release := sync.OnceFunc(func() { close(cleanupRelease) })
		defer release()
		fixture.fake.onStart = func() error {
			launchedAt = time.Now()
			return nil
		}
		fixture.fake.onClose = func(ctx context.Context) error {
			cleanupBegan = time.Now()
			close(cleanupStarted)
			select {
			case <-cleanupRelease:
				cleanupEnded = time.Now()
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		runtime, err := fixture.prepare(t, nil)
		require.NoError(t, err)
		require.True(t, runtime.startedAt.IsZero(), "staging a contract is not command execution")
		source, err := os.ReadFile(fixture.source)
		require.NoError(t, err)
		beforeStart := time.Now()
		require.NoError(t, runtime.Start(context.Background()))
		fixture.waitText(t, "Ready")
		require.Equal(t, true, fixture.act(t, step)["ok"])
		require.Len(t, fixture.fake.inputs, 1)
		require.Equal(t, "x", fixture.fake.inputs[0]["key"])
		fixture.fake.exit(0, 0)
		select {
		case <-cleanupStarted:
		case <-time.After(3 * time.Second):
			t.Fatal("owned cleanup did not start")
		}
		_, ready := runtime.Result()
		require.False(t, ready, "cleanup must finish before result persistence")
		select {
		case <-runtime.Done():
			t.Fatal("runtime completed before owned cleanup")
		default:
		}
		release()
		result := fixture.outcome(t)
		afterFinish := time.Now()
		startedAt := runtimeTimestamp(t, result, "startedAt")
		finishedAt := runtimeTimestamp(t, result, "finishedAt")
		require.False(t, startedAt.Before(beforeStart))
		require.False(t, startedAt.After(launchedAt))
		require.True(t, startedAt.Equal(runtime.origin))
		require.False(t, finishedAt.Before(cleanupEnded))
		require.False(t, finishedAt.After(afterFinish))
		require.True(t, finishedAt.After(startedAt))
		require.True(t, fixture.fake.closed)
		require.Equal(t, "complete", result["captureStatus"])
		require.Equal(t, "passed", result["caseStatus"])
		require.Equal(t, float64(0), result["exitCode"])
		require.Equal(t, float64(1), result["stepsCompleted"])
		require.Equal(t, false, result["interrupted"])
		require.Equal(t, false, result["cleanupError"])
		duration := result["durationSeconds"].(float64)
		require.Greater(t, duration, float64(0))
		require.LessOrEqual(t, duration, cleanupBegan.Sub(runtime.origin).Seconds())
		require.GreaterOrEqual(t, result["cleanupDurationSeconds"].(float64), cleanupEnded.Sub(cleanupBegan).Seconds())
		for _, path := range []string{"contract.json", "capture/contract.json"} {
			saved, err := os.ReadFile(filepath.Join(fixture.workspace, path))
			require.NoError(t, err)
			require.Equal(t, source, saved)
		}
		hash := sha256.Sum256(source)
		require.Equal(t, hex.EncodeToString(hash[:]), result["contractSha256"])
		for _, name := range []string{"states", "events", "raw"} {
			journal, err := os.ReadFile(filepath.Join(fixture.workspace, "capture", name+".jsonl"))
			require.NoError(t, err)
			require.NotEmpty(t, journal)
			previous := -1.0
			for line := range strings.SplitSeq(strings.TrimSpace(string(journal)), "\n") {
				var record document
				require.NoError(t, json.Unmarshal([]byte(line), &record))
				stamp, ok := record["t"].(float64)
				require.True(t, ok)
				require.GreaterOrEqual(t, stamp, float64(0))
				require.GreaterOrEqual(t, stamp, previous)
				require.LessOrEqual(t, stamp, afterFinish.Sub(runtime.origin).Seconds())
				previous = stamp
			}
		}
		saved, err := os.ReadFile(filepath.Join(fixture.workspace, "result.json"))
		require.NoError(t, err)
		raw, ok := runtime.Result()
		require.True(t, ok)
		require.JSONEq(t, string(raw), string(saved))
		repeated, err := runtime.Finish(context.Background(), "already_finished", true)
		require.NoError(t, err)
		require.Equal(t, raw, repeated, "timestamps must not change when Finish is repeated")
	})
	for _, tc := range []struct {
		name              string
		start             bool
		changedExecutable bool
		launchFailure     bool
		cleanupFailure    bool
		wantStartedAt     bool
	}{
		{name: "finish before start omits an absent start"},
		{name: "rejected executable never launches", start: true, changedExecutable: true},
		{name: "failed launch records only the attempted execution", start: true, launchFailure: true, wantStartedAt: true},
		{name: "interruption retains actual timestamps", start: true, wantStartedAt: true},
		{name: "cleanup failure does not become successful", start: true, cleanupFailure: true, wantStartedAt: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newFixture(t)
			var launchedAt, closedAt time.Time
			launchErr := errors.New("synthetic launch failure")
			fixture.fake.onStart = func() error {
				launchedAt = time.Now()
				if tc.launchFailure {
					return launchErr
				}
				return nil
			}
			fixture.fake.onClose = func(context.Context) error {
				closedAt = time.Now()
				if tc.cleanupFailure {
					return errors.New("synthetic cleanup failure")
				}
				return nil
			}
			runtime, err := fixture.prepare(t, nil)
			require.NoError(t, err)
			if tc.changedExecutable {
				executable := str(fixture.contract["command"].(document)["executable"])
				require.NoError(t, os.WriteFile(executable, []byte("Changed synthetic executable.\n"), 0o700))
			}
			beforeStart := time.Now()
			if tc.start {
				err := runtime.Start(context.Background())
				switch {
				case tc.changedExecutable:
					require.Error(t, err)
					require.Equal(t, "executable_changed", safeFault(err).Code)
				case tc.launchFailure:
					require.ErrorIs(t, err, launchErr)
				default:
					require.NoError(t, err)
					fixture.waitText(t, "Ready")
				}
			}
			raw, err := runtime.Finish(context.Background(), "test_stop", true)
			require.NoError(t, err)
			result := fixture.outcome(t)
			finishedAt := runtimeTimestamp(t, result, "finishedAt")
			require.False(t, finishedAt.Before(closedAt))
			require.False(t, finishedAt.After(time.Now()))
			if tc.wantStartedAt {
				startedAt := runtimeTimestamp(t, result, "startedAt")
				require.False(t, startedAt.Before(beforeStart))
				require.False(t, startedAt.After(launchedAt))
				require.True(t, finishedAt.After(startedAt))
			} else {
				require.NotContains(t, result, "startedAt")
				require.True(t, launchedAt.IsZero())
			}
			require.True(t, fixture.fake.closed)
			require.Equal(t, "failed", result["captureStatus"])
			require.Equal(t, "blocked", result["caseStatus"])
			require.Nil(t, result["exitCode"])
			require.Equal(t, tc.cleanupFailure, result["cleanupError"])
			saved, err := os.ReadFile(filepath.Join(fixture.workspace, "result.json"))
			require.NoError(t, err)
			require.JSONEq(t, string(raw), string(saved))
		})
	}
}
