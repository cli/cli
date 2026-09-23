package preflight

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/cli/cli/v2/cli-exercise/internal/terminal"
	"github.com/stretchr/testify/require"
)

type probeSession struct {
	events chan terminal.Event
	stage  string
	closed int
	inputs []json.RawMessage
}

func (session *probeSession) Events() <-chan terminal.Event       { return session.events }
func (session *probeSession) Reply(context.Context, string) error { return nil }
func (session *probeSession) snapshot(text, raw string, columns, rows int) error {
	width := any(len(text))
	if session.stage == "missing width" {
		width = nil
	}
	data, err := json.Marshal(map[string]any{"cols": columns, "rows": rows, "cursor": []int{0, 0},
		"cursorVisible": true, "cursorStyle": "block", "offset": 0, "totalLines": rows,
		"lines": []any{map[string]any{"spans": []any{map[string]any{"text": text, "width": width, "flags": 0}}}},
	})
	if err != nil {
		return err
	}
	session.events <- terminal.Event{Kind: "data", Data: data, Raw: raw}
	return nil
}
func (session *probeSession) Start(context.Context, terminal.LaunchOptions) error {
	if session.stage == "start failure" {
		return fmt.Errorf("synthetic adapter start failure")
	}
	return session.snapshot("CLI_EXERCISE_READY", "CLI_EXERCISE_READY\r\n", 40, 10)
}
func (session *probeSession) Input(_ context.Context, raw json.RawMessage) error {
	session.inputs = append(session.inputs, raw)
	var action struct{ Type, Key string }
	if err := json.Unmarshal(raw, &action); err != nil {
		return err
	}
	if action.Type == "resize" && session.stage != "missing resize snapshot" {
		return session.snapshot("CLI_EXERCISE_READY", "", 44, 12)
	}
	if action.Key == "enter" {
		raw := "CLI_EXERCISE_ACK\r\n"
		if session.stage == "missing raw acknowledgement" {
			raw = ""
		}
		if err := session.snapshot("CLI_EXERCISE_ACK", raw, 44, 12); err != nil {
			return err
		}
		code := 0
		if session.stage == "failed target" {
			code = 2
		}
		session.events <- terminal.Event{Kind: "exit", ExitCode: &code}
	}
	return nil
}
func (session *probeSession) Close(context.Context) error {
	session.closed++
	close(session.events)
	if session.stage == "cleanup failure" {
		return fmt.Errorf("synthetic cleanup failure")
	}
	return nil
}

func sharedProbeCases(t *testing.T) {
	for _, stage := range []string{"ready", "start failure", "missing width", "missing resize snapshot", "missing raw acknowledgement", "failed target", "cleanup failure"} {
		t.Run(stage, func(t *testing.T) {
			session := &probeSession{events: make(chan terminal.Event, 10), stage: stage}
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			err := checkTerminal(ctx, session, terminal.LaunchOptions{Columns: 40, Rows: 10})
			if stage == "ready" {
				require.NoError(t, err)
				require.Len(t, session.inputs, 6)
				require.JSONEq(t, `{"type":"resize","columns":44,"rows":12}`, string(session.inputs[0]))
			} else {
				require.Error(t, err)
			}
			require.Equal(t, 1, session.closed, "the production adapter must always be closed")
		})
	}
}
