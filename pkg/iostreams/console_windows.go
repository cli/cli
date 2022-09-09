//go:build windows
// +build windows

package iostreams

import (
	"golang.org/x/sys/windows"
)

func enableVirtualTerminalProcessing(fd uintptr) error {
	stdout := windows.Handle(fd)

	var originalMode uint32
	windows.GetConsoleMode(stdout, &originalMode)
	return windows.SetConsoleMode(stdout, originalMode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
}
