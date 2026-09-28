//go:build unix

package cliutil

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// ManagedProcesses reports whether owned process-group cleanup is supported.
func ManagedProcesses() bool { return true }

// CheckPrivate verifies both access mode and owner on supported Unix filesystems.
func CheckPrivate(info os.FileInfo) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Getuid()) || info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("run files and directories must be private to their owner")
	}
	return nil
}

// OwnProcessGroup keeps cancellation scoped to the newly created process group.
func OwnProcessGroup(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if command.Cancel != nil {
		command.Cancel = func() error { return KillOwned(command) }
	}
}

// KillOwned stops only a process group created by OwnProcessGroup.
func KillOwned(command *exec.Cmd) error {
	if command.Process == nil {
		return nil
	}
	err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}

// KillProcessGroup stops a known owned group, including descendants of an exited leader.
func KillProcessGroup(ctx context.Context, pid int) error {
	if pid <= 1 || pid == syscall.Getpgrp() {
		return fmt.Errorf("invalid owned process group")
	}
	signalErr := syscall.Kill(-pid, syscall.SIGKILL)
	if errors.Is(signalErr, syscall.ESRCH) {
		return nil
	}
	if signalErr != nil && !errors.Is(signalErr, syscall.EPERM) {
		return fmt.Errorf("terminate owned process group: %w", signalErr)
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		err := syscall.Kill(-pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		if err != nil && !errors.Is(err, syscall.EPERM) {
			return fmt.Errorf("inspect owned process group: %w", err)
		}
		select {
		case <-ctx.Done():
			return errors.Join(signalErr, fmt.Errorf("owned process group did not stop: %w", ctx.Err()))
		case <-ticker.C:
		}
	}
}
