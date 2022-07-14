//go:build !windows
// +build !windows

package iostreams

import (
	"os"
)

func (s *IOStreams) EnableVirtualTerminalProcessing() error {
	return nil
}

func enableVirtualTerminalProcessing(f *os.File) error {
	return nil
}
