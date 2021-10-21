package cmdutil

import (
	"errors"

	"github.com/AlecAivazis/survey/v2/terminal"
)

// A *FlagError indicates an error processing command-line flags or other arguments.
// Such errors cause the application to display the usage message.
type FlagError struct {
	Err error
}

func (fe *FlagError) Error() string {
	return fe.Err.Error()
}

func (fe *FlagError) Unwrap() error {
	return fe.Err
}

// SilentError is an error that triggers exit code 1 without any error messaging
var SilentError = errors.New("SilentError")

// CancelError signals user-initiated cancellation
var CancelError = errors.New("CancelError")

func IsUserCancellation(err error) bool {
	return errors.Is(err, CancelError) || errors.Is(err, terminal.InterruptErr)
}

func MutuallyExclusive(message string, conditions ...bool) error {
	numTrue := 0
	for _, ok := range conditions {
		if ok {
			numTrue++
		}
	}
	if numTrue > 1 {
		return &FlagError{Err: errors.New(message)}
	}
	return nil
}
