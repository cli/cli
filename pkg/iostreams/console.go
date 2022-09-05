//go:build !windows
// +build !windows

package iostreams

func (s *IOStreams) EnableVirtualTerminalProcessing() error {
	return nil
}

func enableVirtualTerminalProcessing(fd uintptr) error {
	return nil
}
