//go:build windows

package detach

import (
	"syscall"

	"golang.org/x/sys/windows"
)

// SysProcAttr returns attributes that isolate a child from console signals and
// suppress a visible console window for it and its descendants.
func SysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_NO_WINDOW}
}
