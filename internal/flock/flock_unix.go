//go:build !windows

package flock

import (
	"errors"
	"os"
	"syscall"
)

// TryLock attempts to acquire an exclusive, non-blocking flock on the given path.
// Returns the locked file and an unlock function on success. The caller should
// read/write through the returned file to avoid platform differences with
// mandatory locking on Windows.
// Returns ErrLocked if the file is already locked by another process.
func TryLock(path string) (f *os.File, unlock func(), err error) {
	f, err = os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, nil, ErrLocked
		}
		return nil, nil, err
	}
	return f, func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}
