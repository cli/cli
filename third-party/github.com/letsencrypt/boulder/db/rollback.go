package db

import (
	"fmt"
)

// RollbackError is a combination of a database error and the error, if any,
// encountered while trying to rollback the transaction.
type RollbackError struct {
	Err         error
	RollbackErr error
}

// Error implements the error interface
func (re *RollbackError) Error() string {
	if re.RollbackErr == nil {
		return re.Err.Error()
	}
	return fmt.Sprintf("%s (also, while rolling back: %s)", re.Err, re.RollbackErr)
}

// rollback rolls back the provided transaction. If the rollback fails for any
// reason a `RollbackError` error is returned wrapping the original error. If no
// rollback error occurs then the original error is returned.
func rollback(tx Transaction, err error) error {
	if txErr := tx.Rollback(); txErr != nil {
		return &RollbackError{
			Err:         err,
			RollbackErr: txErr,
		}
	}
	return err
}
