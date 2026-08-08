//go:build !windows

package telemetry

import "syscall"

// detachAttrs returns SysProcAttr configured to place the child in its own
// process group so that terminal signals delivered to the parent's group
// (SIGINT, SIGHUP) are not forwarded to the child.
func detachAttrs() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}
