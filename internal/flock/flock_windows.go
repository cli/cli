//go:build windows

package flock

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// TryLock attempts to acquire an exclusive, non-blocking lock on the given path.
// Returns the locked file and an unlock function on success. The caller should
// read/write through the returned file to avoid Windows mandatory lock conflicts.
// Returns ErrLocked if the file is already locked by another process.
func TryLock(path string) (f *os.File, unlock func(), err error) {
	f, err = os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, nil, err
	}
	ol := new(windows.Overlapped)
	handle := windows.Handle(f.Fd())
	err = windows.LockFileEx(
		handle,
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0,
		1, 0,
		ol,
	)
	if err != nil {
		_ = f.Close()
		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return nil, nil, ErrLocked
		}
		return nil, nil, err
	}
	return f, func() {
		_ = windows.UnlockFileEx(handle, 0, 1, 0, ol)
		_ = f.Close()
	}, nil
}
