package flock

import (
	"context"
	"errors"
	"os"
	"time"
)

// ErrLocked is returned when the file is already locked by another process.
// Callers can check for this to distinguish contention from permanent errors.
// This is intended to be an OS-agnostic sentinel error.
var ErrLocked = errors.New("file is locked by another process")

// ErrTimeout is returned by Lock when the context is done before the lock could
// be acquired, for example because another process held it past the caller's
// deadline. This is intended to be an OS-agnostic sentinel error.
var ErrTimeout = errors.New("timed out waiting for file lock")

// lockRetryInterval is how long Lock waits between attempts while another
// process holds the lock.
const lockRetryInterval = 50 * time.Millisecond

// Lock blocks until it acquires an exclusive lock on the given path, retrying
// TryLock with small sleep gaps while another process holds the lock. It stops
// waiting when ctx is done and returns ErrTimeout in that case, so a stuck or
// long-held lock cannot block the caller indefinitely. Any error other than
// contention is returned immediately. On success it returns the locked file and
// an unlock function, with the same semantics as TryLock.
func Lock(ctx context.Context, path string) (f *os.File, unlock func(), err error) {
	for {
		f, unlock, err = TryLock(path)
		if err == nil {
			return f, unlock, nil
		} else if !errors.Is(err, ErrLocked) {
			return nil, nil, err
		}

		select {
		case <-ctx.Done():
			return nil, nil, ErrTimeout
		case <-time.After(lockRetryInterval):
		}
	}
}
