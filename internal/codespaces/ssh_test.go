package codespaces

import (
	"context"
	"fmt"
	"slices"
	"testing"
)

type parseTestCase struct {
	Args       []string
	ParsedArgs []string
	Command    []string
	Error      string
}

func TestParseSSHArgs(t *testing.T) {
	testCases := []parseTestCase{
		{}, // empty test case
		{
			Args:       []string{"-X", "-Y"},
			ParsedArgs: []string{"-X", "-Y"},
			Command:    nil,
		},
		{
			Args:       []string{"-X", "-Y", "-o", "someoption=test"},
			ParsedArgs: []string{"-X", "-Y", "-o", "someoption=test"},
			Command:    nil,
		},
		{
			Args:       []string{"-X", "-Y", "-o", "someoption=test", "somecommand"},
			ParsedArgs: []string{"-X", "-Y", "-o", "someoption=test"},
			Command:    []string{"somecommand"},
		},
		{
			Args:       []string{"-X", "-Y", "-o", "someoption=test", "echo", "test"},
			ParsedArgs: []string{"-X", "-Y", "-o", "someoption=test"},
			Command:    []string{"echo", "test"},
		},
		{
			Args:       []string{"somecommand"},
			ParsedArgs: []string{},
			Command:    []string{"somecommand"},
		},
		{
			Args:       []string{"echo", "test"},
			ParsedArgs: []string{},
			Command:    []string{"echo", "test"},
		},
		{
			Args:       []string{"-v", "echo", "hello", "world"},
			ParsedArgs: []string{"-v"},
			Command:    []string{"echo", "hello", "world"},
		},
		{
			Args:       []string{"-L", "-l"},
			ParsedArgs: []string{"-L", "-l"},
			Command:    nil,
		},
		{
			Args:       []string{"-v", "echo", "-n", "test"},
			ParsedArgs: []string{"-v"},
			Command:    []string{"echo", "-n", "test"},
		},
		{
			Args:       []string{"-v", "echo", "-b", "test"},
			ParsedArgs: []string{"-v"},
			Command:    []string{"echo", "-b", "test"},
		},
		{
			Args:       []string{"-b"},
			ParsedArgs: nil,
			Command:    nil,
			Error:      "flag: -b requires an argument",
		},
	}

	for _, tcase := range testCases {
		args, command, err := ParseSSHArgs(tcase.Args)

		checkParseResult(t, tcase, args, command, err)
	}
}

func TestParseSCPArgs(t *testing.T) {
	testCases := []parseTestCase{
		{}, // empty test case
		{
			Args:       []string{"-X", "-Y"},
			ParsedArgs: []string{"-X", "-Y"},
			Command:    nil,
		},
		{
			Args:       []string{"-X", "-Y", "-o", "someoption=test"},
			ParsedArgs: []string{"-X", "-Y", "-o", "someoption=test"},
			Command:    nil,
		},
		{
			Args:       []string{"-X", "-Y", "-o", "someoption=test", "local/file", "remote:file"},
			ParsedArgs: []string{"-X", "-Y", "-o", "someoption=test"},
			Command:    []string{"local/file", "remote:file"},
		},
		{
			Args:       []string{"-X", "-Y", "-o", "someoption=test", "local/file", "remote:file"},
			ParsedArgs: []string{"-X", "-Y", "-o", "someoption=test"},
			Command:    []string{"local/file", "remote:file"},
		},
		{
			Args:       []string{"local/file", "remote:file"},
			ParsedArgs: []string{},
			Command:    []string{"local/file", "remote:file"},
		},
		{
			Args:       []string{"-c"},
			ParsedArgs: nil,
			Command:    nil,
			Error:      "flag: -c requires an argument",
		},
	}

	for _, tcase := range testCases {
		args, command, err := parseSCPArgs(tcase.Args)

		checkParseResult(t, tcase, args, command, err)
	}
}

func checkParseResult(t *testing.T, tcase parseTestCase, gotArgs, gotCmd []string, gotErr error) {
	if tcase.Error != "" {
		if gotErr == nil {
			t.Errorf("expected error and got nil: %#v", tcase)
		}

		if gotErr.Error() != tcase.Error {
			t.Errorf("error does not match expected error, got: '%s', expected: '%s'", gotErr.Error(), tcase.Error)
		}

		return
	}

	if gotErr != nil {
		t.Errorf("unexpected error: %v on test case: %#v", gotErr, tcase)
		return
	}

	argsStr, parsedArgsStr := fmt.Sprintf("%s", gotArgs), fmt.Sprintf("%s", tcase.ParsedArgs)
	if argsStr != parsedArgsStr {
		t.Errorf("args do not match parsed args. got: '%s', expected: '%s'", argsStr, parsedArgsStr)
	}

	commandStr, parsedCommandStr := fmt.Sprintf("%s", gotCmd), fmt.Sprintf("%s", tcase.Command)
	if commandStr != parsedCommandStr {
		t.Errorf("command does not match parsed command. got: '%s', expected: '%s'", commandStr, parsedCommandStr)
	}
}

// TestNewSSHCommandUsesEndOfOptionsSeparator verifies that the ssh argv
// includes the "--" end-of-options separator immediately before the
// destination, so the destination is always parsed as a positional argument.
func TestNewSSHCommandUsesEndOfOptionsSeparator(t *testing.T) {
	cmd, _, err := newSSHCommand(context.Background(), 1234, "user@localhost", []string{"-v"}, []string{"echo", "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// cmd.Args[0] is the ssh executable path; the rest are arguments.
	args := cmd.Args[1:]

	dashDashIdx := slices.Index(args, "--")
	if dashDashIdx == -1 {
		t.Fatalf("expected ssh args to contain a \"--\" separator, got: %v", args)
	}

	dstIdx := slices.Index(args, "user@localhost")
	if dstIdx == -1 {
		t.Fatalf("expected destination in ssh args, got: %v", args)
	}

	if dashDashIdx+1 != dstIdx {
		t.Errorf("expected \"--\" to immediately precede destination, got args: %v", args)
	}
}

// TestNewSCPCommandUsesEndOfOptionsSeparator verifies that the scp argv
// includes the "--" end-of-options separator before the file arguments,
// so file paths are always parsed as positional arguments.
func TestNewSCPCommandUsesEndOfOptionsSeparator(t *testing.T) {
	cmd, err := newSCPCommand(context.Background(), 1234, "user@localhost", []string{"local/file", "remote:file"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	args := cmd.Args[1:]

	dashDashIdx := slices.Index(args, "--")
	if dashDashIdx == -1 {
		t.Fatalf("expected scp args to contain a \"--\" separator, got: %v", args)
	}

	localIdx := slices.Index(args, "local/file")
	if localIdx == -1 {
		t.Fatalf("expected file arg in scp args, got: %v", args)
	}

	if dashDashIdx > localIdx {
		t.Errorf("expected \"--\" to precede file arguments, got args: %v", args)
	}
}
