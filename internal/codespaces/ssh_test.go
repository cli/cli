package codespaces

import "testing"

func TestParseSSHArgs(t *testing.T) {
	type testCase struct {
		Args       []string
		ParsedArgs []string
		Command    []string
	}

	testCases := []testCase{
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
	}

	for _, tcase := range testCases {
		args, command, err := parseSSHArgs(tcase.Args)
		if err != nil {
			t.Errorf("received unexpected error: %w", err)
		}

		if len(args) != len(tcase.ParsedArgs) {
			t.Fatalf("args do not match length of expected args. %#v, got '%d'", tcase, len(args))
		}
		if len(command) != len(tcase.Command) {
			t.Fatalf("command dooes not match length of expected command. %#v, got '%d'", tcase, len(command))
		}

		for i, arg := range args {
			if arg != tcase.ParsedArgs[i] {
				t.Fatalf("arg does not match expected parsed arg. %v, got '%s'", tcase, arg)
			}
		}
		for i, c := range command {
			if c != tcase.Command[i] {
				t.Fatalf("command does not match expected command. %v, got: '%v'", tcase, command)
			}
		}
	}
}

func TestParseSSHArgsError(t *testing.T) {
	_, _, err := parseSSHArgs([]string{"-X", "test", "-Y"})
	if err == nil {
		t.Error("expected an error for invalid args")
	}
}
