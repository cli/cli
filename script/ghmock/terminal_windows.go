package main

import (
	"context"
	"fmt"
	"os/exec"
)

func runInTerminal(context.Context, *exec.Cmd, []string) (terminalRun, error) {
	return terminalRun{}, fmt.Errorf("terminal mode is not supported on Windows")
}
