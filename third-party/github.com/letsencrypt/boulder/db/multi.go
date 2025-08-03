package db

import (
	"context"
	"fmt"
	"strings"
)

// MultiInserter makes it easy to construct a
// `INSERT INTO table (...) VALUES ...;`
// query which inserts multiple rows into the same table. It can also execute
// the resulting query.
type MultiInserter struct {
	// These are validated by the constructor as containing only characters
	// that are allowed in an unquoted identifier.
	// https://mariadb.com/kb/en/identifier-names/#unquoted
	table  string
	fields []string

	values [][]interface{}
}

// NewMultiInserter creates a new MultiInserter, checking for reasonable table
// name and list of fields.
// Safety: `table` and `fields` must contain only strings that are known at
// compile time. They must not contain user-controlled strings.
func NewMultiInserter(table string, fields []string) (*MultiInserter, error) {
	if len(table) == 0 || len(fields) == 0 {
		return nil, fmt.Errorf("empty table name or fields list")
	}

	err := validMariaDBUnquotedIdentifier(table)
	if err != nil {
		return nil, err
	}
	for _, field := range fields {
		err := validMariaDBUnquotedIdentifier(field)
		if err != nil {
			return nil, err
		}
	}

	return &MultiInserter{
		table:  table,
		fields: fields,
		values: make([][]interface{}, 0),
	}, nil
}

// Add registers another row to be included in the Insert query.
func (mi *MultiInserter) Add(row []interface{}) error {
	if len(row) != len(mi.fields) {
		return fmt.Errorf("field count mismatch, got %d, expected %d", len(row), len(mi.fields))
	}
	mi.values = append(mi.values, row)
	return nil
}

// query returns the formatted query string, and the slice of arguments for
// for borp to use in place of the query's question marks. Currently only
// used by .Insert(), below.
func (mi *MultiInserter) query() (string, []interface{}) {
	var questionsBuf strings.Builder
	var queryArgs []interface{}
	for _, row := range mi.values {
		// Safety: We are interpolating a string that will be used in a SQL
		// query, but we constructed that string in this function and know it
		// consists only of question marks joined with commas.
		fmt.Fprintf(&questionsBuf, "(%s),", QuestionMarks(len(mi.fields)))
		queryArgs = append(queryArgs, row...)
	}

	questions := strings.TrimRight(questionsBuf.String(), ",")

	// Safety: we are interpolating `mi.table` and `mi.fields` into an SQL
	// query. We know they contain, respectively, a valid unquoted identifier
	// and a slice of valid unquoted identifiers because we verified that in
	// the constructor. We know the query overall has valid syntax because we
	// generate it entirely within this function.
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES %s", mi.table, strings.Join(mi.fields, ","), questions)

	return query, queryArgs
}

// Insert inserts all the collected rows into the database represented by
// `queryer`.
func (mi *MultiInserter) Insert(ctx context.Context, db Execer) error {
	if len(mi.values) == 0 {
		return nil
	}

	query, queryArgs := mi.query()
	res, err := db.ExecContext(ctx, query, queryArgs...)
	if err != nil {
		return err
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected != int64(len(mi.values)) {
		return fmt.Errorf("unexpected number of rows inserted: %d != %d", affected, len(mi.values))
	}

	return nil
}
