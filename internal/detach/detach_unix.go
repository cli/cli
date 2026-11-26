//go:build !windows

// Package detach provides platform-specific attributes for subprocesses that
// must survive the parent process exiting or receiving terminal signals.
package detach

import "syscall"

// SysProcAttr returns attributes that place a child in its own process group.
func SysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}
