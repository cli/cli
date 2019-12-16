package command

import (
	"strings"
	"testing"
)

func TestCompletion_bash(t *testing.T) {
	output, err := RunCommand(completionCmd, `completion`)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(output.String(), "complete -o default -F __start_gh gh") {
		t.Errorf("problem in bash completion:\n%s", output)
	}
}

func TestCompletion_zsh(t *testing.T) {
	output, err := RunCommand(completionCmd, `completion -s zsh`)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(output.String(), "#compdef _gh gh") {
		t.Errorf("problem in zsh completion:\n%s", output)
	}
}

func TestCompletion_unsupported(t *testing.T) {
	_, err := RunCommand(completionCmd, `completion -s fish`)
	if err == nil || err.Error() != "unsupported shell type: fish" {
		t.Fatal(err)
	}
}
