package main

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/cli/cli/v2/cli-exercise/internal/cliutil"
	"github.com/stretchr/testify/require"
)

func TestParseOptions(t *testing.T) {
	for _, tc := range []struct {
		name    string
		args    []string
		command string
		err     string
	}{
		{name: "preflight", args: []string{"--skill-root", "/skill", "preflight", "check"}, command: "preflight"},
		{name: "run", args: []string{"--skill-root", "/skill", "run", "--contract", "case"}, command: "run"},
		{name: "client needs no source", args: []string{"client", "--run-dir", "run"}, command: "client"},
		{name: "owned Go probe fixture", args: []string{"probe-fixture"}, command: "probe-fixture"},
		{name: "evidence", args: []string{"--skill-root", "/skill", "evidence"}, command: "evidence"},
		{name: "missing operation", err: "choose preflight"},
		{name: "unknown operation", args: []string{"install"}, err: "unknown helper command"},
		{name: "missing root", args: []string{"run"}, err: "--skill-root is required"},
		{name: "unknown option", args: []string{"--unknown"}, err: "flag provided but not defined"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts, err := parseOptions(tc.args)
			if tc.err != "" {
				require.ErrorContains(t, err, tc.err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.command, opts.command)
		})
	}
}

func TestRun(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		code int
		help bool
	}{
		{name: "help", args: []string{"--help"}, help: true},
		{name: "no operation", code: 2},
		{name: "unsupported operation", args: []string{"update"}, code: 2},
		{name: "missing root", args: []string{"preflight", "check"}, code: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(context.Background(), tc.args, cliutil.Streams{In: &bytes.Buffer{}, Out: &stdout, ErrOut: &stderr})
			require.Equal(t, tc.code, code)
			if tc.help {
				require.Contains(t, stdout.String(), "Usage: cli-exercise")
				require.Empty(t, stderr.String())
				return
			}
			require.Empty(t, stdout.String())
			var message struct {
				OK    bool `json:"ok"`
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			require.NoError(t, json.Unmarshal(stderr.Bytes(), &message))
			require.False(t, message.OK)
			require.Equal(t, "usage", message.Error.Code)
		})
	}
}
