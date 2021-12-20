//go:build !windows
// +build !windows

package iostreams

import (
	"errors"
	"os"
)

func (s *IOStreams) EnableVirtualTerminalProcessing() error {
	return nil
}

func enableVirtualTerminalProcessing(f *os.File) error {
	return errors.New("not implemented")
}
