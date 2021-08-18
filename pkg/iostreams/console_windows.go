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

	return enableVirtualTerminalProcessing(s.originalOut.(*os.File))
}

func enableVirtualTerminalProcessing(f *os.File) error {
	stdout := windows.Handle(f.Fd())

	var originalMode uint32
	windows.GetConsoleMode(stdout, &originalMode)
	return windows.SetConsoleMode(stdout, originalMode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
}
