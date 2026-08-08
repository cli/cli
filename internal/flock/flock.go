package flock

import "errors"

// ErrLocked is returned when the file is already locked by another process.
// Callers can check for this to distinguish contention from permanent errors.
// This is intended to be an OS-agnostic sentinel error.
var ErrLocked = errors.New("file is locked by another process")
