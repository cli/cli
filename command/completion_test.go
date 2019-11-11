package command

import (
	"bytes"
	"strings"
	"testing"
)

func TestCompletion_bash(t *testing.T) {
	out := bytes.Buffer{}
	completionCmd.SetOut(&out)

	RootCmd.SetArgs([]string{"completion"})
	_, err := RootCmd.ExecuteC()
	if err != nil {
		t.Fatal(err)
	}

	outStr := out.String()
	if !strings.Contains(outStr, "complete -o default -F __start_gh gh") {
		t.Errorf("problem in bash completion:\n%s", outStr)
	}
}

func TestCompletion_zsh(t *testing.T) {
	out := bytes.Buffer{}
	completionCmd.SetOut(&out)

	RootCmd.SetArgs([]string{"completion", "-s", "zsh"})
	_, err := RootCmd.ExecuteC()
	if err != nil {
		t.Fatal(err)
	}

	outStr := out.String()
	if !strings.Contains(outStr, "#compdef _gh gh") {
		t.Errorf("problem in zsh completion:\n%s", outStr)
	}
}

func TestCompletion_unsupported(t *testing.T) {
	out := bytes.Buffer{}
	completionCmd.SetOut(&out)

	RootCmd.SetArgs([]string{"completion", "-s", "fish"})
	_, err := RootCmd.ExecuteC()
	if err == nil || err.Error() != "unsupported shell type: fish" {
		t.Fatal(err)
	}
}
