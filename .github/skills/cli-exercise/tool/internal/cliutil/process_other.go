//go:build !unix

package cliutil

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// ManagedProcesses reports whether owned process-group cleanup is supported.
func ManagedProcesses() bool { return false }

// CheckPrivate explicitly rejects platforms without implemented ownership checks.
func CheckPrivate(os.FileInfo) error {
	return fmt.Errorf("private ownership checks are unavailable on this platform")
}

// OwnProcessGroup leaves unsupported process setup unchanged.
func OwnProcessGroup(*exec.Cmd) {}

// KillOwned stops only the explicitly created process on unsupported platforms.
func KillOwned(command *exec.Cmd) error {
	if command.Process == nil {
		return nil
	}
	return command.Process.Kill()
}

// KillProcessGroup rejects platforms without implemented group cleanup.
func KillProcessGroup(context.Context, int) error {
	return fmt.Errorf("owned process-group cleanup is unavailable on this platform")
}
