package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"regexp"

	"github.com/go-sql-driver/mysql"
	"github.com/letsencrypt/borp"
)

// ErrDatabaseOp wraps an underlying err with a description of the operation
// that was being performed when the error occurred (insert, select, select
// one, exec, etc) and the table that the operation was being performed on.
type ErrDatabaseOp struct {
	Op    string
	Table string
	Err   error
}

// Error for an ErrDatabaseOp composes a message with context about the
// operation and table as well as the underlying Err's error message.
func (e ErrDatabaseOp) Error() string {
	// If there is a table, include it in the context
	if e.Table != "" {
		return fmt.Sprintf(
			"failed to %s %s: %s",
			e.Op,
			e.Table,
			e.Err)
	}
	return fmt.Sprintf(
		"failed to %s: %s",
		e.Op,
		e.Err)
}

// Unwrap returns the inner error to allow inspection of error chains.
func (e ErrDatabaseOp) Unwrap() error {
	return e.Err
}

// IsNoRows is a utility function for determining if an error wraps the go sql
// package's ErrNoRows, which is returned when a Scan operation has no more
// results to return, and as such is returned by many borp methods.
func IsNoRows(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}

// IsDuplicate is a utility function for determining if an error wrap MySQL's
// Error 1062: Duplicate entry. This error is returned when inserting a row
// would violate a unique key constraint.
func IsDuplicate(err error) bool {
	var dbErr *mysql.MySQLError
	return errors.As(err, &dbErr) && dbErr.Number == 1062
}

// WrappedMap wraps a *borp.DbMap such that its major functions wrap error
// results in ErrDatabaseOp instances before returning them to the caller.
type WrappedMap struct {
	dbMap *borp.DbMap
}

func NewWrappedMap(dbMap *borp.DbMap) *WrappedMap {
	return &WrappedMap{dbMap: dbMap}
}

func (m *WrappedMap) TableFor(t reflect.Type, checkPK bool) (*borp.TableMap, error) {
	return m.dbMap.TableFor(t, checkPK)
}

func (m *WrappedMap) Get(ctx context.Context, holder interface{}, keys ...interface{}) (interface{}, error) {
	return WrappedExecutor{sqlExecutor: m.dbMap}.Get(ctx, holder, keys...)
}

func (m *WrappedMap) Insert(ctx context.Context, list ...interface{}) error {
	return WrappedExecutor{sqlExecutor: m.dbMap}.Insert(ctx, list...)
}

func (m *WrappedMap) Update(ctx context.Context, list ...interface{}) (int64, error) {
	return WrappedExecutor{sqlExecutor: m.dbMap}.Update(ctx, list...)
}

func (m *WrappedMap) Delete(ctx context.Context, list ...interface{}) (int64, error) {
	return WrappedExecutor{sqlExecutor: m.dbMap}.Delete(ctx, list...)
}

func (m *WrappedMap) Select(ctx context.Context, holder interface{}, query string, args ...interface{}) ([]interface{}, error) {
	return WrappedExecutor{sqlExecutor: m.dbMap}.Select(ctx, holder, query, args...)
}

func (m *WrappedMap) SelectOne(ctx context.Context, holder interface{}, query string, args ...interface{}) error {
	return WrappedExecutor{sqlExecutor: m.dbMap}.SelectOne(ctx, holder, query, args...)
}

func (m *WrappedMap) SelectNullInt(ctx context.Context, query string, args ...interface{}) (sql.NullInt64, error) {
	return WrappedExecutor{sqlExecutor: m.dbMap}.SelectNullInt(ctx, query, args...)
}

func (m *WrappedMap) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return WrappedExecutor{sqlExecutor: m.dbMap}.QueryContext(ctx, query, args...)
}

func (m *WrappedMap) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return WrappedExecutor{sqlExecutor: m.dbMap}.QueryRowContext(ctx, query, args...)
}

func (m *WrappedMap) SelectStr(ctx context.Context, query string, args ...interface{}) (string, error) {
	return WrappedExecutor{sqlExecutor: m.dbMap}.SelectStr(ctx, query, args...)
}

func (m *WrappedMap) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return WrappedExecutor{sqlExecutor: m.dbMap}.ExecContext(ctx, query, args...)
}

func (m *WrappedMap) BeginTx(ctx context.Context) (Transaction, error) {
	tx, err := m.dbMap.BeginTx(ctx)
	if err != nil {
		return tx, ErrDatabaseOp{
			Op:  "begin transaction",
			Err: err,
		}
	}
	return WrappedTransaction{
		transaction: tx,
	}, err
}

// WrappedTransaction wraps a *borp.Transaction such that its major functions
// wrap error results in ErrDatabaseOp instances before returning them to the
// caller.
type WrappedTransaction struct {
	transaction *borp.Transaction
}

func (tx WrappedTransaction) Commit() error {
	return tx.transaction.Commit()
}

func (tx WrappedTransaction) Rollback() error {
	return tx.transaction.Rollback()
}

func (tx WrappedTransaction) Get(ctx context.Context, holder interface{}, keys ...interface{}) (interface{}, error) {
	return (WrappedExecutor{sqlExecutor: tx.transaction}).Get(ctx, holder, keys...)
}

func (tx WrappedTransaction) Insert(ctx context.Context, list ...interface{}) error {
	return (WrappedExecutor{sqlExecutor: tx.transaction}).Insert(ctx, list...)
}

func (tx WrappedTransaction) Update(ctx context.Context, list ...interface{}) (int64, error) {
	return (WrappedExecutor{sqlExecutor: tx.transaction}).Update(ctx, list...)
}

func (tx WrappedTransaction) Delete(ctx context.Context, list ...interface{}) (int64, error) {
	return (WrappedExecutor{sqlExecutor: tx.transaction}).Delete(ctx, list...)
}

func (tx WrappedTransaction) Select(ctx context.Context, holder interface{}, query string, args ...interface{}) ([]interface{}, error) {
	return (WrappedExecutor{sqlExecutor: tx.transaction}).Select(ctx, holder, query, args...)
}

func (tx WrappedTransaction) SelectOne(ctx context.Context, holder interface{}, query string, args ...interface{}) error {
	return (WrappedExecutor{sqlExecutor: tx.transaction}).SelectOne(ctx, holder, query, args...)
}

func (tx WrappedTransaction) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return (WrappedExecutor{sqlExecutor: tx.transaction}).QueryContext(ctx, query, args...)
}

func (tx WrappedTransaction) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return (WrappedExecutor{sqlExecutor: tx.transaction}).ExecContext(ctx, query, args...)
}

// WrappedExecutor wraps a borp.SqlExecutor such that its major functions
// wrap error results in ErrDatabaseOp instances before returning them to the
// caller.
type WrappedExecutor struct {
	sqlExecutor borp.SqlExecutor
}

func errForOp(operation string, err error, list []interface{}) ErrDatabaseOp {
	table := "unknown"
	if len(list) > 0 {
		table = fmt.Sprintf("%T", list[0])
	}
	return ErrDatabaseOp{
		Op:    operation,
		Table: table,
		Err:   err,
	}
}

func errForQuery(query, operation string, err error, list []interface{}) ErrDatabaseOp {
	// Extract the table from the query
	table := tableFromQuery(query)
	if table == "" && len(list) > 0 {
		// If there's no table from the query but there was a list of holder types,
		// use the type from the first element of the list and indicate we failed to
		// extract a table from the query.
		table = fmt.Sprintf("%T (unknown table)", list[0])
	} else if table == "" {
		// If there's no table from the query and no list of holders then all we can
		// say is that the table is unknown.
		table = "unknown table"
	}

	return ErrDatabaseOp{
		Op:    operation,
		Table: table,
		Err:   err,
	}
}

func (we WrappedExecutor) Get(ctx context.Context, holder interface{}, keys ...interface{}) (interface{}, error) {
	res, err := we.sqlExecutor.Get(ctx, holder, keys...)
	if err != nil {
		return res, errForOp("get", err, []interface{}{holder})
	}
	return res, err
}

func (we WrappedExecutor) Insert(ctx context.Context, list ...interface{}) error {
	err := we.sqlExecutor.Insert(ctx, list...)
	if err != nil {
		return errForOp("insert", err, list)
	}
	return nil
}

func (we WrappedExecutor) Update(ctx context.Context, list ...interface{}) (int64, error) {
	updatedRows, err := we.sqlExecutor.Update(ctx, list...)
	if err != nil {
		return updatedRows, errForOp("update", err, list)
	}
	return updatedRows, err
}

func (we WrappedExecutor) Delete(ctx context.Context, list ...interface{}) (int64, error) {
	deletedRows, err := we.sqlExecutor.Delete(ctx, list...)
	if err != nil {
		return deletedRows, errForOp("delete", err, list)
	}
	return deletedRows, err
}

func (we WrappedExecutor) Select(ctx context.Context, holder interface{}, query string, args ...interface{}) ([]interface{}, error) {
	result, err := we.sqlExecutor.Select(ctx, holder, query, args...)
	if err != nil {
		return result, errForQuery(query, "select", err, []interface{}{holder})
	}
	return result, err
}

func (we WrappedExecutor) SelectOne(ctx context.Context, holder interface{}, query string, args ...interface{}) error {
	err := we.sqlExecutor.SelectOne(ctx, holder, query, args...)
	if err != nil {
		return errForQuery(query, "select one", err, []interface{}{holder})
	}
	return nil
}

func (we WrappedExecutor) SelectNullInt(ctx context.Context, query string, args ...interface{}) (sql.NullInt64, error) {
	rows, err := we.sqlExecutor.SelectNullInt(ctx, query, args...)
	if err != nil {
		return sql.NullInt64{}, errForQuery(query, "select", err, nil)
	}
	return rows, nil
}

func (we WrappedExecutor) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	// Note: we can't do error wrapping here because the error is passed via the `*sql.Row`
	// object, and we can't produce a `*sql.Row` object with a custom error because it is unexported.
	return we.sqlExecutor.QueryRowContext(ctx, query, args...)
}

func (we WrappedExecutor) SelectStr(ctx context.Context, query string, args ...interface{}) (string, error) {
	str, err := we.sqlExecutor.SelectStr(ctx, query, args...)
	if err != nil {
		return "", errForQuery(query, "select", err, nil)
	}
	return str, nil
}

func (we WrappedExecutor) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	rows, err := we.sqlExecutor.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, errForQuery(query, "select", err, nil)
	}
	return rows, nil
}

var (
	// selectTableRegexp matches the table name from an SQL select statement
	selectTableRegexp = regexp.MustCompile(`(?i)^\s*select\s+[a-z\d:\.\(\), \_\*` + "`" + `]+\s+from\s+([a-z\d\_,` + "`" + `]+)`)
	// insertTableRegexp matches the table name from an SQL insert statement
	insertTableRegexp = regexp.MustCompile(`(?i)^\s*insert\s+into\s+([a-z\d \_,` + "`" + `]+)\s+(?:set|\()`)
	// updateTableRegexp matches the table name from an SQL update statement
	updateTableRegexp = regexp.MustCompile(`(?i)^\s*update\s+([a-z\d \_,` + "`" + `]+)\s+set`)
	// deleteTableRegexp matches the table name from an SQL delete statement
	deleteTableRegexp = regexp.MustCompile(`(?i)^\s*delete\s+from\s+([a-z\d \_,` + "`" + `]+)\s+where`)

	// tableRegexps is a list of regexps that tableFromQuery will try to use in
	// succession to find the table name for an SQL query. While tableFromQuery
	// isn't used by the higher level borp Insert/Update/Select/etc functions we
	// include regexps for matching inserts, updates, selects, etc because we want
	// to match the correct table when these types of queries are run through
	// ExecContext().
	tableRegexps = []*regexp.Regexp{
		selectTableRegexp,
		insertTableRegexp,
		updateTableRegexp,
		deleteTableRegexp,
	}
)

// tableFromQuery uses the tableRegexps on the provided query to return the
// associated table name or an empty string if it can't be determined from the
// query.
func tableFromQuery(query string) string {
	for _, r := range tableRegexps {
		if matches := r.FindStringSubmatch(query); len(matches) >= 2 {
			return matches[1]
		}
	}
	return ""
}

func (we WrappedExecutor) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	res, err := we.sqlExecutor.ExecContext(ctx, query, args...)
	if err != nil {
		return res, errForQuery(query, "exec", err, args)
	}
	return res, nil
}
