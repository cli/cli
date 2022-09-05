//go:build windows
// +build windows

package iostreams

import (
	"os"

	"golang.org/x/sys/windows"
)

func (s *IOStreams) EnableVirtualTerminalProcessing() error {
	if !s.IsStdoutTTY() {
		return nil
	}

	return enableVirtualTerminalProcessing(s.Out.Fd())
}

func enableVirtualTerminalProcessing(fd uintptr) error {
	stdout := windows.Handle(fd)

	var originalMode uint32
	windows.GetConsoleMode(stdout, &originalMode)
	return windows.SetConsoleMode(stdout, originalMode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
}
