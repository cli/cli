//go:build !windows

package main

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// containCommand gives a command and its Git children one process group so
// cancelling a run does not leave a transport or pack process behind. A PTY
// creates its own session instead.
func containCommand(cmd *exec.Cmd, terminal bool) {
	if !terminal {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	cmd.WaitDelay = 5 * time.Second
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
}
