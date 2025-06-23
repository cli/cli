package db

import (
	"context"
	"testing"

	berrors "github.com/letsencrypt/boulder/errors"
	"github.com/letsencrypt/boulder/test"
)

func TestRollback(t *testing.T) {
	ctx := context.Background()
	dbMap := testDbMap(t)

	tx, _ := dbMap.BeginTx(ctx)
	// Commit the transaction so that a subsequent rollback will always fail.
	_ = tx.Commit()

	innerErr := berrors.NotFoundError("Gone, gone, gone")
	result := rollback(tx, innerErr)

	// Since the tx.Rollback will fail we expect the result to be a wrapped error
	test.AssertNotEquals(t, result, innerErr)
	if rbErr, ok := result.(*RollbackError); ok {
		test.AssertEquals(t, rbErr.Err, innerErr)
		test.AssertNotNil(t, rbErr.RollbackErr, "RollbackErr was nil")
	} else {
		t.Fatalf("Result was not a RollbackError: %#v", result)
	}

	// Create a new transaction and don't commit it this time. The rollback should
	// succeed.
	tx, _ = dbMap.BeginTx(ctx)
	result = rollback(tx, innerErr)

	// We expect that the err is returned unwrapped.
	test.AssertEquals(t, result, innerErr)
}
