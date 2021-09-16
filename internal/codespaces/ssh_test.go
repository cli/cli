package codespaces

import "testing"

func TestParseSSHArgs(t *testing.T) {
	type testCase struct {
		Args       []string
		ParsedArgs []string
		Command    string
	}

	testCases := []testCase{
		{
			Args:       []string{"-X", "-Y"},
			ParsedArgs: []string{"-X", "-Y"},
			Command:    "",
		},
		{
			Args:       []string{"-X", "-Y", "-o", "someoption=test"},
			ParsedArgs: []string{"-X", "-Y", "-o", "someoption=test"},
			Command:    "",
		},
		{
			Args:       []string{"-X", "-Y", "-o", "someoption=test", "somecommand"},
			ParsedArgs: []string{"-X", "-Y", "-o", "someoption=test"},
			Command:    "somecommand",
		},
		{
			Args:       []string{"-X", "-Y", "-o", "someoption=test", "echo", "test"},
			ParsedArgs: []string{"-X", "-Y", "-o", "someoption=test"},
			Command:    "echo test",
		},
		{
			Args:       []string{"somecommand"},
			ParsedArgs: []string{},
			Command:    "somecommand",
		},
		{
			Args:       []string{"echo", "test"},
			ParsedArgs: []string{},
			Command:    "echo test",
		},
		{
			Args:       []string{"-v", "echo", "hello", "world"},
			ParsedArgs: []string{"-v"},
			Command:    "echo hello world",
		},
	}

	for _, tcase := range testCases {
		args, command, err := parseSSHArgs(tcase.Args)
		if err != nil {
			t.Errorf("received unexpected error: %w", err)
		}

		if len(args) != len(tcase.ParsedArgs) {
			t.Fatalf("args do not match length of expected args. %#v, got '%d', expected: '%d'", tcase, len(args), len(tcase.ParsedArgs))
		}
		for i, arg := range args {
			if arg != tcase.ParsedArgs[i] {
				t.Fatalf("arg does not match expected parsed arg. %v, got '%s', expected: '%s'", tcase, arg, tcase.ParsedArgs[i])
			}
		}
		if command != tcase.Command {
			t.Fatalf("command does not match expected command. %v, got: '%s', expected: '%s'", tcase, command, tcase.Command)
		}
	}
}

func TestParseSSHArgsError(t *testing.T) {
	_, _, err := parseSSHArgs([]string{"-X", "test", "-Y"})
	if err == nil {
		t.Error("expected an error for invalid args")
	}
}
