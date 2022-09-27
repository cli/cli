//go:build !windows
// +build !windows

package iostreams

import "errors"

func enableVirtualTerminalProcessing(fd uintptr) error {
	return errors.New("not implemented")
}
