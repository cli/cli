package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func denialKeyCases(t *testing.T) {
	for _, tc := range []struct {
		name              string
		denied, requested any
		allowed           bool
		code              string
	}{
		{name: "return denial blocks enter", denied: "return", requested: "enter"},
		{name: "enter denial blocks return", denied: "enter", requested: "return"},
		{name: "escape denial blocks esc", denied: "escape", requested: "esc"},
		{name: "esc denial blocks escape", denied: "esc", requested: "escape"},
		{name: "linefeed denial blocks enter", denied: "linefeed", requested: "enter"},
		{name: "control M denial blocks return", denied: []any{"ctrl", "m"}, requested: "return"},
		{name: "return denial blocks control M", denied: "return", requested: []any{"ctrl", "m"}},
		{name: "return denial blocks uppercase control M", denied: "return", requested: []any{"ctrl", "M"}},
		{name: "uppercase control M denial blocks return", denied: []any{"ctrl", "M"}, requested: "return"},
		{name: "enter denial blocks uppercase control J", denied: "enter", requested: []any{"ctrl", "J"}},
		{name: "tab denial blocks control I", denied: "tab", requested: []any{"ctrl", "i"}},
		{name: "tab denial blocks uppercase control I", denied: "tab", requested: []any{"ctrl", "I"}},
		{name: "control H denial blocks backspace", denied: []any{"ctrl", "h"}, requested: "backspace"},
		{name: "uppercase control H denial blocks backspace", denied: []any{"ctrl", "H"}, requested: "backspace"},
		{name: "modifier order does not change a denial", denied: []any{"ctrl", "alt", "enter"}, requested: []any{"alt", "ctrl", "enter"}},
		{name: "uppercase literal denial blocks shifted letter", denied: "M", requested: []any{"shift", "m"}},
		{name: "shifted letter denial blocks uppercase literal", denied: []any{"shift", "m"}, requested: "M"},
		{name: "shifted alt denial blocks uppercase alt", denied: []any{"shift", "alt", "m"}, requested: []any{"alt", "M"}},
		{name: "ignored Meta is rejected", denied: "enter", requested: []any{"meta", "enter"}, code: "invalid_action"},
		{name: "ignored Alt on control character is rejected", denied: "enter", requested: []any{"alt", "ctrl", "M"}, code: "invalid_action"},
		{name: "ignored Shift on control character is rejected", denied: "enter", requested: []any{"shift", "ctrl", "M"}, code: "invalid_action"},
		{name: "ignored Ctrl on punctuation is rejected", denied: "[", requested: []any{"ctrl", "["}, code: "invalid_action"},
		{name: "ignored Ctrl on arrow is rejected", denied: "up", requested: []any{"ctrl", "up"}, code: "invalid_action"},
		{name: "ignored Shift on arrow is rejected", denied: "up", requested: []any{"shift", "up"}, code: "invalid_action"},
		{name: "ignored Shift on digit is rejected", denied: "1", requested: []any{"shift", "1"}, code: "invalid_action"},
		{name: "different key remains allowed", denied: "return", requested: "x", allowed: true},
		{name: "additional modifier remains distinct", denied: "return", requested: []any{"alt", "enter"}, allowed: true},
		{name: "shift preserves distinct uppercase input", denied: "m", requested: []any{"shift", "m"}, allowed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newFixture(t)
			fixture.contract["mode"] = "explore"
			fixture.contract["constraints"] = document{"deny": []any{document{
				"action": document{"type": "key", "key": tc.denied}, "reason": "The fixture forbids this physical key.",
			}}}
			fixture.start(t, nil)
			before, err := os.ReadFile(filepath.Join(fixture.workspace, "contract.json"))
			require.NoError(t, err)
			response := fixture.act(t, document{"type": "key", "key": tc.requested})
			if tc.allowed {
				require.Equal(t, true, response["ok"], response)
				require.Len(t, fixture.fake.inputs, 1)
			} else {
				code := tc.code
				if code == "" {
					code = "action_denied"
				}
				require.Equal(t, code, errorCode(response))
				require.Empty(t, fixture.fake.inputs)
			}
			after, err := os.ReadFile(filepath.Join(fixture.workspace, "contract.json"))
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
	for _, tc := range []struct {
		name                           string
		supplied, equivalent, physical any
	}{
		{name: "return", supplied: "return", equivalent: "enter", physical: "enter"},
		{name: "uppercase control", supplied: []any{"ctrl", "M"}, equivalent: []any{"ctrl", "m"}, physical: []any{"ctrl", "m"}},
		{name: "shifted letter", supplied: []any{"shift", "m"}, equivalent: "M", physical: "M"},
	} {
		t.Run("exact steps keep their original spelling/"+tc.name, func(t *testing.T) {
			fixture := newFixture(t)
			fixture.contract["steps"] = []any{document{"type": "key", "key": tc.supplied}}
			fixture.start(t, nil)
			require.Equal(t, "exact_mismatch", errorCode(fixture.act(t, document{"type": "key", "key": tc.equivalent})))
			require.Equal(t, true, fixture.act(t, document{"type": "key", "key": tc.supplied})["ok"])
			require.Equal(t, tc.supplied, fixture.runtime.contract.steps[0]["key"])
			require.Equal(t, tc.physical, fixture.fake.inputs[0]["key"])
		})
	}
	t.Run("alias denial also applies to text newlines", func(t *testing.T) {
		fixture := newFixture(t)
		fixture.contract["mode"] = "explore"
		fixture.contract["constraints"] = document{"deny": []any{document{"action": document{"key": "return"}}}}
		fixture.start(t, nil)
		require.Equal(t, "action_denied", errorCode(fixture.act(t, document{"type": "text", "text": "\n"})))
		require.Empty(t, fixture.fake.inputs)
	})
}

func keyValidationCases(t *testing.T) {
	for _, tc := range []struct {
		name    string
		key     any
		invalid bool
	}{
		{name: "uppercase control letter", key: []any{"ctrl", "M"}},
		{name: "shifted letter", key: []any{"shift", "m"}},
		{name: "shifted uppercase letter", key: []any{"shift", "M"}},
		{name: "modified special key", key: []any{"ctrl", "alt", "shift", "enter"}},
		{name: "Alt arrow", key: []any{"alt", "up"}},
		{name: "Meta", key: []any{"meta", "enter"}, invalid: true},
		{name: "Alt control character", key: []any{"alt", "ctrl", "M"}, invalid: true},
		{name: "Shift control character", key: []any{"shift", "ctrl", "M"}, invalid: true},
		{name: "Ctrl punctuation", key: []any{"ctrl", "["}, invalid: true},
		{name: "Ctrl space", key: []any{"ctrl", "space"}, invalid: true},
		{name: "Ctrl linefeed", key: []any{"ctrl", "linefeed"}, invalid: true},
		{name: "Ctrl arrow", key: []any{"ctrl", "up"}, invalid: true},
		{name: "Shift arrow", key: []any{"shift", "up"}, invalid: true},
		{name: "Shift digit", key: []any{"shift", "1"}, invalid: true},
		{name: "Shift punctuation", key: []any{"shift", "["}, invalid: true},
		{name: "Shift function key", key: []any{"shift", "f1"}, invalid: true},
	} {
		for _, location := range []string{"step", "denial"} {
			t.Run(tc.name+"/"+location, func(t *testing.T) {
				fixture := newFixture(t)
				action := document{"type": "key", "key": tc.key}
				if location == "step" {
					fixture.contract["steps"] = []any{action}
				} else {
					fixture.contract["constraints"] = document{"deny": []any{document{"action": action}}}
				}
				_, err := fixture.prepare(t, nil)
				if tc.invalid {
					require.Error(t, err)
					require.Equal(t, "invalid_action", safeFault(err).Code)
				} else {
					require.NoError(t, err)
				}
				require.Empty(t, fixture.fake.inputs)
			})
		}
	}
}

func controllerDeadlineCases(t *testing.T) {
	for _, name := range []string{"before input", "during typing", "waiting for acknowledgment", "during navigation", "waiting for a rule"} {
		t.Run(name, func(t *testing.T) {
			fixture := newFixture(t)
			action := document{"type": "key", "key": "x"}
			condition := document{}
			partial := true
			switch name {
			case "before input":
				action["timing"] = document{"delayBeforeMs": 1000}
				partial = false
			case "during typing":
				action = document{"type": "text", "text": "abc", "timing": document{"typingDelayMs": 300}}
			case "waiting for acknowledgment":
				fixture.fake.onInputContext = func(ctx context.Context, _ document) error {
					<-ctx.Done()
					return ctx.Err()
				}
			case "during navigation":
				action = document{"type": "select", "label": "Blue"}
			case "waiting for a rule":
				condition["screenContains"] = "Not reached"
				partial = false
			}
			fixture.contract["steps"] = []any{action}
			fixture.start(t, nil)
			if name == "during navigation" {
				fixture.fake.emit("Choose\n> Red\n  Blue", "")
				fixture.waitText(t, "Choose")
			}
			observed := fixture.request(t, document{"op": "observe"})
			raw, err := json.Marshal(document{
				"op": "run_controller", "revision": observed["revision"], "reason": "Exercise a bounded controller.",
				"controller": document{"maxDurationSeconds": .1, "waitForMatchMs": 3000, "rules": []any{
					document{"id": "bounded", "when": condition, "action": action, "reason": "Only act within the time budget."},
				}},
			})
			require.NoError(t, err)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			start := time.Now()
			response, err := fixture.runtime.Handle(ctx, raw)
			require.NoError(t, err)
			require.Less(t, time.Since(start), 500*time.Millisecond, "the controller deadline must cover its actions")
			var value document
			require.NoError(t, json.Unmarshal(response, &value))
			if partial {
				require.Equal(t, "controller_limit", errorCode(value), value)
				result := fixture.outcome(t)
				require.Equal(t, "blocked", result["caseStatus"])
				require.Equal(t, float64(0), result["stepsCompleted"])
				require.Len(t, fixture.fake.inputs, 1)
			} else {
				require.Equal(t, "paused", value["controllerStatus"])
				require.Equal(t, "controller_limit", value["reason"])
				require.Empty(t, fixture.fake.inputs)
			}
		})
	}
}

func durationCancellationCase(t *testing.T) {
	fixture := newFixture(t)
	fixture.contract["steps"] = []any{document{"type": "key", "key": "x"}}
	fixture.contract["limits"].(document)["maxDurationSeconds"] = .2
	fixture.fake.onInputContext = func(ctx context.Context, _ document) error {
		<-ctx.Done()
		return ctx.Err()
	}
	fixture.start(t, nil)
	observed := fixture.request(t, document{"op": "observe"})
	raw, err := json.Marshal(document{"op": "act", "revision": observed["revision"],
		"reason": "Exercise pending-input cleanup.", "action": document{"type": "key", "key": "x"}})
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	start := time.Now()
	_, err = fixture.runtime.Handle(ctx, raw)
	require.NoError(t, err)
	require.Less(t, time.Since(start), time.Second, "the runtime limit must cancel pending input before waiting for the action lock")
	result := fixture.outcome(t)
	require.Equal(t, "duration_limit", result["stopReason"])
	require.Equal(t, "blocked", result["caseStatus"])
}
