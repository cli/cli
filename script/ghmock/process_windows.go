package main

import (
	"os/exec"
	"time"
)

func containCommand(cmd *exec.Cmd, _ bool) {
	cmd.WaitDelay = 5 * time.Second
}
