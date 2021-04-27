// +build windows

package iostreams

import (
	"os"

	"golang.org/x/sys/windows"
)

func (s *IOStreams) EnableVirtualTerminalProcessing() {
	if !s.IsStdoutTTY() {
		return
	}

	stdout := windows.Handle(s.originalOut.(*os.File).Fd())

	var originalMode uint32
	windows.GetConsoleMode(stdout, &originalMode)
	windows.SetConsoleMode(stdout, originalMode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
}
