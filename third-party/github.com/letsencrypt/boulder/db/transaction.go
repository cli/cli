package db

import "context"

// txFunc represents a function that does work in the context of a transaction.
type txFunc func(tx Executor) (interface{}, error)

// WithTransaction runs the given function in a transaction, rolling back if it
// returns an error and committing if not. The provided context is also attached
// to the transaction. WithTransaction also passes through a value returned by
// `f`, if there is no error.
func WithTransaction(ctx context.Context, dbMap DatabaseMap, f txFunc) (interface{}, error) {
	tx, err := dbMap.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	result, err := f(tx)
	if err != nil {
		return nil, rollback(tx, err)
	}
	err = tx.Commit()
	if err != nil {
		return nil, err
	}
	return result, nil
}
