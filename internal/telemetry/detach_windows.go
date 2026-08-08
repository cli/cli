//go:build windows

package telemetry

import (
	"syscall"

	"golang.org/x/sys/windows"
)

// detachAttrs returns SysProcAttr configured to place the child in its own
// process group so that console signals (Ctrl+C) delivered to the parent's
// group are not forwarded to the child, and to suppress any console window
// for the child and its descendants.
//
// CREATE_NO_WINDOW is preferred over DETACHED_PROCESS here: DETACHED_PROCESS
// removes the console entirely, which causes any console-subsystem descendant
// (e.g. tzutil.exe invoked transitively to resolve the local IANA timezone)
// to allocate a fresh conhost window, producing a visible flash on every gh
// invocation. CREATE_NO_WINDOW gives the child a non-visible console that
// descendants can inherit, avoiding the flash.
func detachAttrs() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_NO_WINDOW}
}
