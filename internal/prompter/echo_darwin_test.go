//go:build darwin

package prompter_test

import "golang.org/x/sys/unix"

//nolint:unused // ioctl number kept for planned echo-mode polling tests
const ioctlGetTermios = unix.TIOCGETA
