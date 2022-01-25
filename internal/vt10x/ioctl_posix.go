//go:build linux || darwin || dragonfly || solaris || openbsd || netbsd || freebsd
// +build linux darwin dragonfly solaris openbsd netbsd freebsd

package vt10x

import (
	"os"
	"syscall"
	"unsafe"
)

func ioctl(f *os.File, cmd, p uintptr) error {
	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		f.Fd(),
		syscall.TIOCSWINSZ,
		p)
	if errno != 0 {
		return syscall.Errno(errno)
	}
	return nil
}

func ResizePty(pty *os.File, cols, rows int) error {
	var w struct{ row, col, xpix, ypix uint16 }
	w.row = uint16(rows)
	w.col = uint16(cols)
	w.xpix = 16 * uint16(cols)
	w.ypix = 16 * uint16(rows)
	return ioctl(pty, syscall.TIOCSWINSZ,
		uintptr(unsafe.Pointer(&w)))
}
