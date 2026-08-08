//go:build darwin

package prompter_test

import "golang.org/x/sys/unix"

const ioctlGetTermios = unix.TIOCGETA
