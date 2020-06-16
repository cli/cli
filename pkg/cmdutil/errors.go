package cmdutil

import "errors"

// FlagError is the kind of error raised in flag processing
type FlagError struct {
	Err error
}

func (fe FlagError) Error() string {
	return fe.Err.Error()
}

func (fe FlagError) Unwrap() error {
	return fe.Err
}

// SilentError is an error that triggers exit code 1 without any error messaging
var SilentError = errors.New("SilentError")
