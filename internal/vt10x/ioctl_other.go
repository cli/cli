//go:build plan9 || nacl || windows
// +build plan9 nacl windows

package vt10x

import (
	"os"
)

func ioctl(f *os.File, cmd, p uintptr) error {
	return nil
}

func ResizePty(pty *os.File, cols, rows int) error {
	return nil
}
