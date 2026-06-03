package cmdutil

import (
	"errors"
	"fmt"

	"github.com/AlecAivazis/survey/v2/terminal"
)

// FlagErrorf returns a new FlagError that wraps an error produced by
// fmt.Errorf(format, args...).
func FlagErrorf(format string, args ...interface{}) error {
	return FlagErrorWrap(fmt.Errorf(format, args...))
}

// FlagErrorWrap returns a new FlagError that wraps the specified error.
func FlagErrorWrap(err error) error { return &FlagError{err} }

// A *FlagError indicates an error processing command-line flags or other arguments.
// Such errors cause the application to display the usage message.
type FlagError struct {
	// Note: not struct{error}: only *FlagError should satisfy error.
	err error
}

func (fe *FlagError) Error() string {
	return fe.err.Error()
}

func (fe *FlagError) Unwrap() error {
	return fe.err
}

// SilentError is an error that triggers exit code 1 without any error messaging
var SilentError = errors.New("SilentError")

// CancelError signals user-initiated cancellation
var CancelError = errors.New("CancelError")

// PendingError signals nothing failed but something is pending
var PendingError = errors.New("PendingError")

// PRAlreadyExistsError signals that a pull request already exists.
// This uses exit code 32 so scripts can distinguish it from other errors.
type PRAlreadyExistsError struct {
	Message string
}

func (e *PRAlreadyExistsError) Error() string {
	return e.Message
}

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
		return FlagErrorf("%s", message)
	}
	return nil
}

type NoResultsError struct {
	message string
}

func (e NoResultsError) Error() string {
	return e.message
}

func NewNoResultsError(message string) NoResultsError {
	return NoResultsError{message: message}
}
