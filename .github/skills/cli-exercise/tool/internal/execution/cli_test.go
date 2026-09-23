package execution

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/cli/cli/v2/cli-exercise/internal/cliutil"
	"github.com/cli/cli/v2/cli-exercise/internal/recording"
	"github.com/stretchr/testify/require"
)

func TestParseOptions(t *testing.T) {
	for _, tc := range []struct {
		name, kind string
		args       []string
		fail       bool
	}{
		{name: "run", kind: "run", args: []string{"--contract", "case", "--preflight", "ready"}},
		{name: "run missing receipt", kind: "run", args: []string{"--contract", "case"}, fail: true},
		{name: "client stdin", kind: "client", args: []string{"--run-dir", "workspace"}},
		{name: "client file", kind: "client", args: []string{"--run-dir", "workspace", "--file", "request"}},
		{name: "client timeout", kind: "client", args: []string{"--run-dir", "workspace", "--timeout", "NaN"}, fail: true},
		{name: "client missing workspace", kind: "client", fail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var err error
			if tc.kind == "run" {
				_, err = parseRun(tc.args)
			} else {
				_, err = parseClient(tc.args)
			}
			if tc.fail {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestRun(t *testing.T) {
	for _, tc := range []struct {
		name, op string
		want     int
	}{
		{name: "closed observe", op: "observe"},
		{name: "closed finish", op: "finish"},
		{name: "closed shutdown", op: "shutdown"},
		{name: "closed input is not submitted", op: "act", want: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			require.NoError(t, os.Chmod(root, 0o700))
			require.NoError(t, cliutil.WriteJSON(filepath.Join(root, "runtime.json"), runtimeStatus{Status: "finished"}))
			require.NoError(t, cliutil.WriteJSON(filepath.Join(root, "contract.json"), map[string]any{
				"environment": map[string]any{"pass": []any{}},
			}))
			require.NoError(t, cliutil.WriteJSON(filepath.Join(root, "result.json"), map[string]any{"caseStatus": "passed"}))
			var output bytes.Buffer
			code, err := clientCommand(context.Background(), clientOptions{directory: root, timeout: 1}, cliutil.Streams{
				In: bytes.NewBufferString(`{"op":"` + tc.op + `"}`), Out: &output, ErrOut: io.Discard,
			})
			if tc.want != 0 {
				require.Error(t, err)
				require.Equal(t, tc.want, code)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, code)
			require.Contains(t, output.String(), `"ok":true`)
		})
	}
	t.Run("native Go owner and thin Node adapter", func(t *testing.T) {
		helper := os.Getenv("CLI_EXERCISE_TEST_HELPER")
		receiptPath := os.Getenv("CLI_EXERCISE_TEST_RECEIPT")
		if helper == "" || receiptPath == "" {
			t.Skip("Select an existing built helper and genuine ready receipt for native integration.")
		}
		skillRoot, err := filepath.Abs("../../..")
		require.NoError(t, err)
		var receipt recording.Receipt
		_, err = cliutil.ReadJSON(receiptPath, 4<<20, &receipt)
		require.NoError(t, err)
		fixture, err := os.Executable()
		require.NoError(t, err)
		fixtureHash, err := cliutil.SHA256File(fixture)
		require.NoError(t, err)
		for _, mode := range []string{"exact", "explore"} {
			t.Run(mode, func(t *testing.T) {
				base := t.TempDir()
				workspace := filepath.Join(base, "run")
				steps := []any{}
				if mode == "exact" {
					steps = []any{
						map[string]any{"type": "text", "text": "Mona"},
						map[string]any{"type": "key", "key": "enter"},
						map[string]any{"type": "select", "label": "Blue"},
					}
				}
				source := map[string]any{
					"schemaVersion": 1, "caseId": mode, "mode": mode,
					"source": map[string]any{"kind": "synthetic", "text": "Complete only this local fixture for Mona and Blue."},
					"goal":   "Complete the synthetic fixture for Mona and Blue.", "workspace": workspace,
					"command": map[string]any{"executable": fixture, "args": []string{},
						"sha256": fixtureHash, "version": "synthetic Go test fixture", "cwd": "work"},
					"environment":   map[string]any{"values": map[string]string{"CLI_EXERCISE_TEST_FIXTURE": "workflow", "GORACE": "atexit_sleep_ms=0"}, "pass": []any{}},
					"authorization": map[string]any{"status": "approved", "basis": "The caller selected this local isolated test.", "effects": []any{}},
					"terminal": map[string]any{"columns": 80, "rows": 18, "fontSize": 18, "fps": 30,
						"background": "#0d1117", "foreground": "#e6edf3"},
					"limits": map[string]any{"maxActions": 20, "maxDurationSeconds": 30, "idleTimeoutSeconds": 10},
					"steps":  steps, "constraints": map[string]any{"deny": []any{map[string]any{
						"action": map[string]any{"type": "key", "key": "enter"},
						"when":   map[string]any{"questionPrefix": "Choose a color", "selected": "Red"},
						"reason": "The case must not confirm the default Red selection.",
					}}},
					"expectations": []any{
						map[string]any{"id": "exit", "type": "exit_code", "value": 0},
						map[string]any{"id": "name", "type": "screen_contains", "value": "Hello, Mona."},
						map[string]any{"id": "selection", "type": "screen_contains", "value": "Selected: Blue."},
					},
					"output": map[string]any{"formats": []string{"gif"}},
				}
				if mode == "explore" {
					source["output"] = map[string]any{"formats": []string{"gif", "mp4"}, "timing": "realtime"}
				}
				contractPath := filepath.Join(base, "input.json")
				require.NoError(t, cliutil.WriteJSON(contractPath, source))
				contractHash, err := cliutil.SHA256File(contractPath)
				require.NoError(t, err)
				ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
				defer cancel()
				command := exec.CommandContext(ctx, helper, "--skill-root", skillRoot, "run", "--contract", contractPath, "--preflight", receiptPath)
				command.Dir, command.Env = base, []string{}
				var logs bytes.Buffer
				command.Stdout, command.Stderr = &logs, &logs
				require.NoError(t, command.Start())
				finished := false
				defer func() {
					if !finished {
						_ = command.Process.Signal(os.Interrupt)
						_ = command.Wait()
					}
				}()
				deadline := time.Now().Add(5 * time.Second)
				for {
					var status runtimeStatus
					_, err := cliutil.ReadJSON(filepath.Join(workspace, "runtime.json"), 4096, &status)
					if err == nil && status.Status == "ready" {
						break
					}
					require.True(t, time.Now().Before(deadline), "Go owner did not become ready")
					time.Sleep(10 * time.Millisecond)
				}
				send := func(request map[string]any, fromFile bool) map[string]any {
					raw, err := json.Marshal(request)
					require.NoError(t, err)
					args := []string{"client", "--run-dir", workspace, "--timeout", "5"}
					if fromFile {
						file := filepath.Join(base, "request.json")
						require.NoError(t, os.WriteFile(file, raw, 0o600))
						args = append(args, "--file", file)
					}
					client := exec.CommandContext(ctx, helper, args...)
					client.Env, client.Dir = []string{}, base
					client.Stdin = bytes.NewReader(raw)
					response, err := client.CombinedOutput()
					require.NoError(t, err, string(response))
					var value map[string]any
					require.NoError(t, json.Unmarshal(response, &value))
					require.Equal(t, true, value["ok"], value)
					return value
				}
				observed := send(map[string]any{"op": "wait", "until": map[string]any{"screenContains": "Name:"}, "timeoutMs": 2000}, false)
				if mode == "exact" {
					send(map[string]any{"op": "act", "revision": observed["revision"], "reason": "Enter the supplied name.", "action": steps[0]}, true)
					observed = send(map[string]any{"op": "wait", "until": map[string]any{"screenContains": "Name: Mona"}, "timeoutMs": 2000}, false)
					send(map[string]any{"op": "act", "revision": observed["revision"], "reason": "Complete the supplied name.", "action": steps[1]}, false)
					observed = send(map[string]any{"op": "wait", "until": map[string]any{"screenContains": "Choose a color"}, "timeoutMs": 2000}, false)
				} else {
					observed = send(map[string]any{
						"op": "run_controller", "revision": observed["revision"], "reason": "Use rules only for the known name prompt.",
						"controller": map[string]any{"waitForMatchMs": 100, "rules": []any{
							map[string]any{"id": "name", "when": map[string]any{"screenContains": "Name:"}, "action": map[string]any{"type": "text", "text": "Mona"}, "reason": "Supply the goal's name."},
							map[string]any{"id": "continue", "when": map[string]any{"screenContains": "Name: Mona"}, "action": map[string]any{"type": "key", "key": "enter"}, "reason": "Reveal the next state."},
						}},
					}, false)
					require.Equal(t, float64(2), observed["actions"])
					require.Equal(t, "unhandled_state", observed["reason"])
					for _, tc := range []struct {
						key  any
						code string
					}{
						{key: []string{"ctrl", "M"}, code: "action_denied"},
						{key: []string{"meta", "enter"}, code: "invalid_action"},
						{key: []string{"alt", "ctrl", "M"}, code: "invalid_action"},
					} {
						raw, err := json.Marshal(map[string]any{
							"op": "act", "revision": observed["revision"], "reason": "Verify the denied confirmation is not delivered.",
							"action": map[string]any{"type": "key", "key": tc.key},
						})
						require.NoError(t, err)
						client := exec.CommandContext(ctx, helper, "client", "--run-dir", workspace, "--timeout", "5")
						client.Env, client.Dir, client.Stdin = []string{}, base, bytes.NewReader(raw)
						var output, errors bytes.Buffer
						client.Stdout, client.Stderr = &output, &errors
						require.Error(t, client.Run(), output.String())
						var response struct {
							OK    bool                  `json:"ok"`
							Error struct{ Code string } `json:"error"`
						}
						require.NoError(t, json.Unmarshal(output.Bytes(), &response), errors.String())
						require.False(t, response.OK)
						require.Equal(t, tc.code, response.Error.Code)
						observed = send(map[string]any{"op": "observe"}, false)
						view := observed["observation"].(map[string]any)
						require.Equal(t, false, view["exited"], "a denied Enter must not confirm the real terminal selection")
						require.Equal(t, "Red", view["menu"].(map[string]any)["selected"])
					}
				}
				send(map[string]any{"op": "act", "revision": observed["revision"], "reason": "Choose the required Blue outcome.",
					"action": map[string]any{"type": "select", "label": "Blue"}}, false)
				err = command.Wait()
				finished = true
				require.NoError(t, err, logs.String())
				var result map[string]any
				_, err = cliutil.ReadJSON(filepath.Join(workspace, "result.json"), 4<<20, &result)
				require.NoError(t, err)
				require.Equal(t, "passed", result["caseStatus"], result)
				require.Equal(t, "complete", result["captureStatus"])
				require.Equal(t, false, result["cleanupError"])
				if mode == "exact" {
					require.Equal(t, float64(3), result["stepsCompleted"])
				}
				after := send(map[string]any{"op": "observe"}, false)
				require.NotNil(t, after["result"])
				render := exec.CommandContext(ctx, helper, "--skill-root", skillRoot, "evidence",
					"--run-dir", workspace, "--preflight", receiptPath, "--inspection", "all")
				render.Env, render.Dir = []string{}, base
				rendered, err := render.CombinedOutput()
				require.NoError(t, err, string(rendered))
				var evidence struct {
					CaseStatus string `json:"caseStatus"`
					Rendering  struct {
						Status     string            `json:"status"`
						Timing     string            `json:"timing"`
						Media      map[string]string `json:"media"`
						Frames     int               `json:"frames"`
						Inspection struct {
							Manifest     string `json:"manifest"`
							UniqueImages int    `json:"uniqueImages"`
						} `json:"inspection"`
					} `json:"rendering"`
				}
				require.NoError(t, json.Unmarshal(rendered, &evidence))
				require.Equal(t, "passed", evidence.CaseStatus)
				require.Equal(t, "complete", evidence.Rendering.Status)
				wantTiming := "condensed"
				if mode == "explore" {
					wantTiming = "realtime"
				}
				require.Equal(t, wantTiming, evidence.Rendering.Timing)
				savedHash, err := cliutil.SHA256File(filepath.Join(workspace, "contract.json"))
				require.NoError(t, err)
				require.Equal(t, contractHash, savedHash, "timing defaults must not rewrite the execution contract")
				require.FileExists(t, evidence.Rendering.Media["gif"])
				var manifest struct {
					Frames   map[string]string `json:"frames"`
					Rendered []struct {
						Image string `json:"image"`
						First int    `json:"firstFrame"`
						Last  int    `json:"lastFrame"`
						State int    `json:"sourceState"`
					} `json:"rendered"`
					SourceStates []struct {
						Image string `json:"image"`
						State int    `json:"state"`
					} `json:"sourceStates"`
				}
				_, err = cliutil.ReadJSON(evidence.Rendering.Inspection.Manifest, 4<<20, &manifest)
				require.NoError(t, err)
				next := 0
				for _, occurrence := range manifest.Rendered {
					require.Equal(t, next, occurrence.First)
					require.GreaterOrEqual(t, occurrence.Last, occurrence.First)
					require.Contains(t, manifest.Frames, occurrence.Image)
					require.Less(t, occurrence.State, len(manifest.SourceStates))
					next = occurrence.Last + 1
				}
				require.Equal(t, evidence.Rendering.Frames, next, "deduplication must preserve every video frame")
				for index, occurrence := range manifest.SourceStates {
					require.Equal(t, index, occurrence.State)
					require.Contains(t, manifest.Frames, occurrence.Image)
				}
				require.Less(t, evidence.Rendering.Inspection.UniqueImages, len(manifest.Rendered)+len(manifest.SourceStates))
				if mode == "explore" {
					require.FileExists(t, evidence.Rendering.Media["mp4"])
				} else {
					require.NotContains(t, evidence.Rendering.Media, "mp4")
				}
			})
		}
	})
}
