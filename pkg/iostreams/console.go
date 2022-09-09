//go:build !windows
// +build !windows

package iostreams

func enableVirtualTerminalProcessing(fd uintptr) error {
	return nil
}
