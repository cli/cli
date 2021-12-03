package frecency

import (
	"database/sql"
	"errors"
	"time"

	"github.com/cli/cli/v2/api"
	_ "github.com/mattn/go-sqlite3"
)

// stores issue/PR with frecency stats
type entryWithStats struct {
	Entry api.Issue
	Stats countEntry
}

type countEntry struct {
	LastAccess time.Time
	Count      int
}

func updateEntry(db *sql.DB, updated *entryWithStats, repoName string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare("UPDATE issues SET lastAccess = ?, count = ? WHERE repo = ? AND number = ?")
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()
	_, err = stmt.Exec(updated.Stats.LastAccess.Unix(), updated.Stats.Count, repoName, updated.Entry.Number)
	if err != nil {
		tx.Rollback()
		return err
	}
	tx.Commit()
	return nil
}

func insertEntry(db *sql.DB, repoName string, entry *entryWithStats, isPR bool) error {
	// insert the repo if it doesn't exist yet
	repoExists, err := repoExists(db, repoName)
	if err != nil {
		return err
	}
	if !repoExists {
		if err := insertRepo(db, repoName); err != nil {
			return err
		}
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare("INSERT INTO issues(title,number,count,lastAccess,repo,isPR) values(?,?,?,?,?,?)")
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(
		entry.Entry.Title,
		entry.Entry.Number,
		entry.Stats.Count,
		entry.Stats.LastAccess.Unix(),
		repoName,
		isPR)
	if err != nil {
		tx.Rollback()
		return err
	}
	tx.Commit()
	return nil
}

func insertRepo(db *sql.DB, repoName string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare("INSERT INTO repos(fullName) values(?)")
	if err != nil {
		tx.Rollback()
		return err
	}

	defer stmt.Close()
	_, err = stmt.Exec(repoName)
	if err != nil {
		tx.Rollback()
		return err
	}

	tx.Commit()
	return nil
}

func repoExists(db *sql.DB, repoName string) (bool, error) {
	var found int
	row := db.QueryRow("SELECT 1 FROM repos WHERE fullName = ?", repoName)
	err := row.Scan(&found)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return false, err
}

// get issues or PRs
func getEntries(db *sql.DB, repoName string, isPR bool) ([]entryWithStats, error) {
	query := `
	SELECT number,lastAccess,count,title FROM issues
		WHERE repo = ?
		AND isPR = ?
		ORDER BY lastAccess DESC`
	rows, err := db.Query(query, repoName, isPR)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := []entryWithStats{}
	for rows.Next() {
		var entry entryWithStats
		var lastAccess int64
		err := rows.Scan(&entry.Entry.Number, &lastAccess, &entry.Stats.Count, &entry.Entry.Title)
		if err != nil {
			return nil, err
		}
		entry.Stats.LastAccess = time.Unix(lastAccess, 0)
		entries = append(entries, entry)
	}
	return entries, nil
}

func createTables(db *sql.DB) error {
	// TODO: repo is identified by "owner/repo",
	// so renaming and transfering ownership will invalidate the db
	query := `
	CREATE TABLE IF NOT EXISTS repos(
		id INTEGER PRIMARY KEY AUTOINCREMENT, 
 		fullName TEXT NOT NULL UNIQUE,
		issuesLastQueried INTEGER,
		prsLastQueried INTEGER
	);
	
	CREATE TABLE IF NOT EXISTS issues(
		id INTEGER PRIMARY KEY AUTOINCREMENT, 
		title TEXT NOT NULL,
		number INTEGER NOT NULL,
		count INTEGER NOT NULL,
		lastAccess INTEGER NOT NULL,
		isPR BOOLEAN NOT NULL 
			CHECK (isPR IN (0,1))
			DEFAULT 0,
		repo TEXT NOT NULL,
		FOREIGN KEY (repo) REFERENCES repo(fullName)
	);

	CREATE INDEX IF NOT EXISTS 
	frecent ON issues(lastAccess, count);
	`
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	if _, err = tx.Exec(query); err != nil {
		return err
	}

	tx.Commit()
	return nil
}
