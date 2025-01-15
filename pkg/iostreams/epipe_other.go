//go:build !windows
// +build !windows

package iostreams

import (
	"errors"
	"syscall"
)

func isEpipeError(err error) bool {
	return errors.Is(err, syscall.EPIPE)
}
