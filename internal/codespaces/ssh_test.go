package codespaces

import (
	"fmt"
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
		args, command, err := parseSSHArgs(tcase.Args)

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
