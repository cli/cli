//go:build unix

package safepaths

import (
	"os"
	"syscall"
)

func nonBlockingOpenFlag() int {
	return syscall.O_NONBLOCK
}

func clearNonblocking(file *os.File) error {
	return syscall.SetNonblock(int(file.Fd()), false)
}
