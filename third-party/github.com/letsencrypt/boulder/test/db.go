package test

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"testing"
)

var (
	_ CleanUpDB = &sql.DB{}
)

// CleanUpDB is an interface with only what is needed to delete all
// rows in all tables in a database plus close the database
// connection. It is satisfied by *sql.DB.
type CleanUpDB interface {
	BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error)
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
	QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error)

	io.Closer
}

// ResetBoulderTestDatabase returns a cleanup function which deletes all rows in
// all tables of the 'boulder_sa_test' database. Omits the 'gorp_migrations'
// table as this is used by sql-migrate (https://github.com/rubenv/sql-migrate)
// to track migrations. If it encounters an error it fails the tests.
func ResetBoulderTestDatabase(t testing.TB) func() {
	return resetTestDatabase(t, context.Background(), "boulder")
}

// ResetIncidentsTestDatabase returns a cleanup function which deletes all rows
// in all tables of the 'incidents_sa_test' database. Omits the
// 'gorp_migrations' table as this is used by sql-migrate
// (https://github.com/rubenv/sql-migrate) to track migrations. If it encounters
// an error it fails the tests.
func ResetIncidentsTestDatabase(t testing.TB) func() {
	return resetTestDatabase(t, context.Background(), "incidents")
}

func resetTestDatabase(t testing.TB, ctx context.Context, dbPrefix string) func() {
	db, err := sql.Open("mysql", fmt.Sprintf("test_setup@tcp(boulder-proxysql:6033)/%s_sa_test", dbPrefix))
	if err != nil {
		t.Fatalf("Couldn't create db: %s", err)
	}
	err = deleteEverythingInAllTables(ctx, db)
	if err != nil {
		t.Fatalf("Failed to delete everything: %s", err)
	}
	return func() {
		err := deleteEverythingInAllTables(ctx, db)
		if err != nil {
			t.Fatalf("Failed to truncate tables after the test: %s", err)
		}
		_ = db.Close()
	}
}

// clearEverythingInAllTables deletes all rows in the tables
// available to the CleanUpDB passed in and resets the autoincrement
// counters. See allTableNamesInDB for what is meant by "all tables
// available". To be used only in test code.
func deleteEverythingInAllTables(ctx context.Context, db CleanUpDB) error {
	ts, err := allTableNamesInDB(ctx, db)
	if err != nil {
		return err
	}
	for _, tn := range ts {
		// We do this in a transaction to make sure that the foreign
		// key checks remain disabled even if the db object chooses
		// another connection to make the deletion on. Note that
		// `alter table` statements will silently cause transactions
		// to commit, so we do them outside of the transaction.
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("unable to start transaction to delete all rows from table %#v: %s", tn, err)
		}
		_, err = tx.ExecContext(ctx, "set FOREIGN_KEY_CHECKS = 0")
		if err != nil {
			return fmt.Errorf("unable to disable FOREIGN_KEY_CHECKS to delete all rows from table %#v: %s", tn, err)
		}
		// 1 = 1 here prevents the MariaDB i_am_a_dummy setting from
		// rejecting the DELETE for not having a WHERE clause.

		_, err = tx.ExecContext(ctx, "delete from `"+tn+"` where 1 = 1")
		if err != nil {
			return fmt.Errorf("unable to delete all rows from table %#v: %s", tn, err)
		}
		_, err = tx.ExecContext(ctx, "set FOREIGN_KEY_CHECKS = 1")
		if err != nil {
			return fmt.Errorf("unable to re-enable FOREIGN_KEY_CHECKS to delete all rows from table %#v: %s", tn, err)
		}
		err = tx.Commit()
		if err != nil {
			return fmt.Errorf("unable to commit transaction to delete all rows from table %#v: %s", tn, err)
		}

		_, err = db.ExecContext(ctx, "alter table `"+tn+"` AUTO_INCREMENT = 1")
		if err != nil {
			return fmt.Errorf("unable to reset autoincrement on table %#v: %s", tn, err)
		}
	}
	return err
}

// allTableNamesInDB returns the names of the tables available to the passed
// CleanUpDB. Omits the 'gorp_migrations' table as this is used by sql-migrate
// (https://github.com/rubenv/sql-migrate) to track migrations.
func allTableNamesInDB(ctx context.Context, db CleanUpDB) ([]string, error) {
	r, err := db.QueryContext(ctx, "select table_name from information_schema.tables t where t.table_schema = DATABASE() and t.table_name != 'gorp_migrations';")
	if err != nil {
		return nil, err
	}
	var ts []string
	for r.Next() {
		tableName := ""
		err = r.Scan(&tableName)
		if err != nil {
			return nil, err
		}
		ts = append(ts, tableName)
	}
	return ts, r.Err()
}
