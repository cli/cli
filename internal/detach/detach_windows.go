//go:build windows

package detach

import (
	"syscall"

	"golang.org/x/sys/windows"
)

// SysProcAttr returns attributes that isolate a child from console signals and
// suppress a visible console window for it and its descendants. CREATE_NO_WINDOW
// is used instead of DETACHED_PROCESS so console-subsystem descendants inherit
// a hidden console rather than allocating a briefly visible one.
func SysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_NO_WINDOW}
}
