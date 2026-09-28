// Package terminal provides the mechanical Tuistory bridge used by probes and execution.
package terminal

import (
	"context"
	"encoding/json"
)

// LaunchOptions describes one explicitly selected, isolated target invocation.
type LaunchOptions struct {
	Executable string            `json:"executable"`
	Args       []string          `json:"args"`
	Cwd        string            `json:"cwd"`
	Env        map[string]string `json:"env"`
	Columns    int               `json:"columns"`
	Rows       int               `json:"rows"`
}

// Event carries actual terminal output or target lifecycle information.
type Event struct {
	Kind     string
	Data     json.RawMessage
	Raw      string
	ExitCode *int
	Signal   *int
	Err      error
}

// Session is a mechanical terminal boundary without case policy.
// Events remain ordered, and Close waits for the owned adapter and target to stop.
type Session interface {
	Start(context.Context, LaunchOptions) error
	Events() <-chan Event
	Input(context.Context, json.RawMessage) error
	Reply(context.Context, string) error
	Close(context.Context) error
}
