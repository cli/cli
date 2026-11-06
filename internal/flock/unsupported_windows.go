//go:build windows

package flock

import (
	"errors"

	"golang.org/x/sys/windows"
)

// IsUnsupported reports whether the filesystem does not support file locking.
func IsUnsupported(err error) bool {
	return errors.Is(err, windows.ERROR_NOT_SUPPORTED) ||
		errors.Is(err, windows.ERROR_INVALID_FUNCTION)
}
