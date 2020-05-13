package command

import (
	"strings"
	"testing"
)

func TestCompletion_bash(t *testing.T) {
	output, err := RunCommand(`completion`)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(output.String(), "complete -o default -F __start_gh gh") {
		t.Errorf("problem in bash completion:\n%s", output)
	}
}

func TestCompletion_zsh(t *testing.T) {
	output, err := RunCommand(`completion -s zsh`)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(output.String(), "#compdef _gh gh") {
		t.Errorf("problem in zsh completion:\n%s", output)
	}
}

func TestCompletion_fish(t *testing.T) {
	output, err := RunCommand(`completion -s fish`)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(output.String(), "complete -c gh ") {
		t.Errorf("problem in fish completion:\n%s", output)
	}
}

func TestCompletion_powerShell(t *testing.T) {
	output, err := RunCommand(`completion -s powershell`)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(output.String(), "Register-ArgumentCompleter") {
		t.Errorf("problem in powershell completion:\n%s", output)
	}
}

func TestCompletion_unsupported(t *testing.T) {
	_, err := RunCommand(`completion -s csh`)
	if err == nil || err.Error() != `unsupported shell type "csh"` {
		t.Fatal(err)
	}
}
