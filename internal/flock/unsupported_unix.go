//go:build !windows

package flock

import (
	"errors"
	"syscall"
)

// IsUnsupported reports whether the filesystem does not support file locking.
func IsUnsupported(err error) bool {
	return errors.Is(err, syscall.ENOTSUP) ||
		errors.Is(err, syscall.EOPNOTSUPP) ||
		errors.Is(err, syscall.ENOSYS)
}
