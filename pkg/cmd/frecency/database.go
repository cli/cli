package frecency

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/cli/cli/v2/api"
	_ "github.com/mattn/go-sqlite3"
)

// entryWithStats stores a issue or PR with frecency stats.
// NOTE: frecency uses entryWithStats both for getting database entries
// and as arguments for database queries
type entryWithStats struct {
	IsPR     bool
	RepoName string // full name OWNER/REPO
	Entry    api.Issue
	Stats    countEntry
}

type countEntry struct {
	LastAccess time.Time
	Count      int
}

func createTables(db *sql.DB) error {
	// TODO: repo is identified by "owner/repo",
	// so renaming and transfering ownership will invalidate the db
	query := `
	CREATE TABLE IF NOT EXISTS repos(
		id INTEGER PRIMARY KEY AUTOINCREMENT, 
 		fullName TEXT NOT NULL, 
		issuesLastQueried INTEGER DEFAULT 0,
		prsLastQueried INTEGER DEFAULT 0,
		UNIQUE(fullName) ON CONFLICT IGNORE
	);
	
	CREATE TABLE IF NOT EXISTS issues(
		id INTEGER PRIMARY KEY AUTOINCREMENT, 
		gqlID TEXT NOT NULL,
		title TEXT NOT NULL,
		number INTEGER NOT NULL,
		count INTEGER NOT NULL,
		lastAccess INTEGER NOT NULL,
		isPR BOOLEAN NOT NULL 
			CHECK (isPR IN (0,1))
			DEFAULT 0,
		repo TEXT NOT NULL,
		UNIQUE(repo,number) ON CONFLICT REPLACE,
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

	return tx.Commit()
}

// update the frecency stats of an entry
func updateEntry(db *sql.DB, updated entryWithStats) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare("UPDATE issues SET lastAccess = ?, count = ? WHERE repo = ? AND number = ?")
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer stmt.Close()
	_, err = stmt.Exec(updated.Stats.LastAccess.Unix(), updated.Stats.Count, updated.RepoName, updated.Entry.Number)
	if err != nil {
		return tx.Rollback()
	}
	return tx.Commit()
}

func insertEntry(db *sql.DB, entry entryWithStats) error {
	if err := insertRepo(db, entry.RepoName); err != nil {
		return err
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare("INSERT INTO issues(title,number,count,lastAccess,repo,isPR,gqlID) values(?,?,?,?,?,?,?)")
	if err != nil {
		return tx.Rollback()
	}
	defer stmt.Close()

	_, err = stmt.Exec(
		entry.Entry.Title,
		entry.Entry.Number,
		entry.Stats.Count,
		entry.Stats.LastAccess.Unix(),
		entry.RepoName,
		entry.IsPR,
		entry.Entry.ID)

	if err != nil {
		return tx.Rollback()
	}
	return tx.Commit()
}

func insertRepo(db *sql.DB, repoName string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare("INSERT INTO repos(fullName) values(?)")
	if err != nil {
		return tx.Rollback()
	}

	defer stmt.Close()
	_, err = stmt.Exec(repoName)
	if err != nil {
		return tx.Rollback()
	}

	return tx.Commit()
}

// get all issues or PRs by repo, sorted by most recent
func getEntries(db *sql.DB, repoDetails entryWithStats) ([]entryWithStats, error) {
	query := `
	SELECT number,lastAccess,count,title,gqlID FROM issues
		WHERE repo = ?
		AND isPR = ?
		ORDER BY lastAccess DESC, number DESC`
	rows, err := db.Query(query, repoDetails.RepoName, repoDetails.IsPR)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := []entryWithStats{}
	for rows.Next() {
		entry := entryWithStats{IsPR: repoDetails.IsPR, Stats: countEntry{}}
		var unixTime int64
		if err := rows.Scan(&entry.Entry.Number, &unixTime, &entry.Stats.Count, &entry.Entry.Title, &entry.Entry.ID); err != nil {
			return nil, err
		}
		entry.Stats.LastAccess = time.Unix(unixTime, 0)
		entry.RepoName = repoDetails.RepoName
		entries = append(entries, entry)
	}
	return entries, nil
}

// lookup single issue or PR by number
func getEntryByNumber(db *sql.DB, repoDetails entryWithStats) (entryWithStats, error) {
	query := `
	SELECT number,title,lastAccess,count,isPR,gqlID FROM issues
		WHERE repo = ? AND number = ?`
	rows, err := db.Query(query, repoDetails.RepoName, repoDetails.Entry.Number)
	if err != nil {
		return entryWithStats{}, err
	}
	defer rows.Close()

	entry := entryWithStats{}

	for rows.Next() {
		var unixTime int64
		err = rows.Scan(
			&entry.Entry.Number,
			&entry.Entry.Title,
			&unixTime,
			&entry.Stats.Count,
			&entry.IsPR,
			&entry.Entry.ID)

		if err != nil {
			return entry, err
		}
		entry.Stats.LastAccess = time.Unix(unixTime, 0)
		entry.RepoName = repoDetails.RepoName
	}
	return entry, nil
}

// get last query time for a repo's Issues or PRs
func getLastQueried(db *sql.DB, repoDetails entryWithStats) (time.Time, error) {
	field := "issuesLastQueried"
	if repoDetails.IsPR {
		field = "prsLastQueried"
	}
	query := fmt.Sprintf("SELECT %s from repos where fullName = ?", field)

	rows, err := db.Query(query, repoDetails.RepoName)
	if err != nil {
		return time.Time{}, err
	}

	var lastQueried time.Time
	for rows.Next() {
		var unixTime int64
		err = rows.Scan(&unixTime)
		if err != nil {
			return lastQueried, err
		}
		lastQueried = time.Unix(unixTime, 0)
	}
	return lastQueried, nil
}

func updateLastQueried(db *sql.DB, repoDetails entryWithStats) error {
	field := "issuesLastQueried"
	if repoDetails.IsPR {
		field = "prsLastQueried"
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}

	query := fmt.Sprintf("UPDATE repos SET %s = ? WHERE fullName = ?", field)
	stmt, err := tx.Prepare(query)
	if err != nil {
		return tx.Rollback()
	}
	defer stmt.Close()
	_, err = stmt.Exec(repoDetails.Stats.LastAccess.Unix(), repoDetails.RepoName)
	if err != nil {
		return tx.Rollback()
	}
	return tx.Commit()
}

func deleteByNumber(db *sql.DB, entry entryWithStats) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare("DELETE FROM issues WHERE repo = ? AND number = ?")
	if err != nil {
		return tx.Rollback()
	}
	defer stmt.Close()
	_, err = stmt.Exec(entry.RepoName, entry.Entry.Number)
	if err != nil {
		return tx.Rollback()
	}
	return tx.Commit()
}

// delete entries with count less than specified number
func deleteByCount(db *sql.DB, entry entryWithStats) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare("DELETE FROM issues WHERE repo = ? AND count < ?")
	if err != nil {
		return tx.Rollback()
	}
	defer stmt.Close()
	_, err = stmt.Exec(entry.RepoName, entry.Stats.Count)
	if err != nil {
		return tx.Rollback()
	}
	return tx.Commit()
}
