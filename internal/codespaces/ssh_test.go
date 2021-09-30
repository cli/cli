package codespaces

import (
	"fmt"
	"testing"
)

func TestParseSSHArgs(t *testing.T) {
	type testCase struct {
		Args       []string
		ParsedArgs []string
		Command    []string
		Error      string
	}

	testCases := []testCase{
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
			Error:      "ssh flag: -b requires an argument",
		},
	}

	for _, tcase := range testCases {
		args, command, err := parseSSHArgs(tcase.Args)
		if tcase.Error != "" {
			if err == nil {
				t.Errorf("expected error and got nil: %#v", tcase)
			}

			if err.Error() != tcase.Error {
				t.Errorf("error does not match expected error, got: '%s', expected: '%s'", err.Error(), tcase.Error)
			}

			continue
		}

		if err != nil {
			t.Errorf("unexpected error: %v on test case: %#v", err, tcase)
			continue
		}

		argsStr, parsedArgsStr := fmt.Sprintf("%s", args), fmt.Sprintf("%s", tcase.ParsedArgs)
		if argsStr != parsedArgsStr {
			t.Errorf("args do not match parsed args. got: '%s', expected: '%s'", argsStr, parsedArgsStr)
		}

		commandStr, parsedCommandStr := fmt.Sprintf("%s", command), fmt.Sprintf("%s", tcase.Command)
		if commandStr != parsedCommandStr {
			t.Errorf("command does not match parsed command. got: '%s', expected: '%s'", commandStr, parsedCommandStr)
		}
	}
}
