package frecency

import (
	"database/sql"
	"errors"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// DBEntry is a frecency entry for a issue or PR
// PRs are also issues, so we can store them in one table
// IDs are the graphQL IDs
type DBEntry struct {
	Number int
	Title  string
	ID     string
	Repo   Repository
	Stats  CountEntry
	IsPR   bool
}

type Repository struct {
	OwnerID           string
	OwnerName         string
	ID                string
	Name              string
	IssuesLastQueried time.Time // the last time issues/PRs were fetched
	PRsLastQueried    time.Time
}

func UpdateEntry(db *sql.DB, updated *DBEntry) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}

	statement, err := tx.Prepare("UPDATE issues SET lastAccessed = ?, count = ? WHERE repoId = ? AND number = ?")
	if err != nil {
		tx.Rollback()
		return err
	}

	defer statement.Close()

	_, err = statement.Exec(
		updated.Stats.LastAccessed.Unix(),
		updated.Stats.Count,
		updated.Repo.ID,
		updated.Number)
	if err != nil {
		tx.Rollback()
		return err
	}
	tx.Commit()
	return nil
}

// Create new Issue/PR entry in database
func insertEntry(db *sql.DB, entry *DBEntry) error {

	// insert the owner if it doesn't exist yet
	ownerExists, err := OwnerExists(db, entry.Repo.OwnerID)
	if err != nil {
		return err
	}
	if !ownerExists {
		if err := insertOwner(db, entry.Repo); err != nil {
			return err
		}
	}

	// insert the repo if it doesn't exist yet
	repoExists, err := RepoExists(db, entry.Repo.ID)
	if err != nil {
		return err
	}
	if !repoExists {
		err := insertRepo(db, &entry.Repo)
		if err != nil {
			return err
		}
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare("INSERT INTO issues(gqlID,title,number,count,lastAccessed,repoID,isPR) values(?,?,?,?,?,?,?)")
	if err != nil {
		tx.Rollback()
		return err
	}

	defer stmt.Close()
	_, err = stmt.Exec(
		entry.ID,
		entry.Title,
		entry.Number,
		entry.Stats.Count,
		entry.Stats.LastAccessed.Unix(),
		entry.Repo.ID,
		entry.IsPR)

	if err != nil {
		tx.Rollback()
		return err
	}
	tx.Commit()
	return nil
}

func insertOwner(db *sql.DB, repository Repository) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}

	statement, err := tx.Prepare("INSERT INTO owners(gqlID,name) values(?,?)")
	if err != nil {
		tx.Rollback()
		return err
	}

	defer statement.Close()
	_, err = statement.Exec(repository.OwnerID, repository.OwnerName)

	if err != nil {
		tx.Rollback()
		return err
	}

	tx.Commit()
	return nil
}

func insertRepo(db *sql.DB, repo *Repository) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}

	statement, err := tx.Prepare("INSERT INTO repos(gqlID,name,ownerID) values(?,?,?)")
	if err != nil {
		tx.Rollback()
		return err
	}

	defer statement.Close()

	_, err = statement.Exec(repo.ID, repo.Name, repo.OwnerID)
	if err != nil {
		tx.Rollback()
		return err
	}

	tx.Commit()
	return nil
}

func OwnerExists(db *sql.DB, ownerID string) (bool, error) {
	row := db.QueryRow("SELECT id FROM owners WHERE gqlID = ?", ownerID)
	var id string
	err := row.Scan(&id)
	if err == nil {
		return true, nil
	}

	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return false, err
}

func RepoExists(db *sql.DB, repoID string) (bool, error) {
	var id string
	row := db.QueryRow("SELECT id FROM repos WHERE gqlID = ?", repoID)
	err := row.Scan(&id)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return false, err
}

// Retrieve all issues under the repo with given ID
func GetIssues(db *sql.DB, repoID string) ([]*DBEntry, error) {
	return getEntries(db, repoID, 0)
}

// Retrieve all PRs under the repo with given ID
func GetPullRequests(db *sql.DB, repoID string) ([]*DBEntry, error) {
	return getEntries(db, repoID, 1)
}

func getEntries(db *sql.DB, repoID string, isPR int) ([]*DBEntry, error) {
	query := `
	SELECT number,lastAccessed,count,isPR,title FROM issues 
		WHERE repoID = ? 
		AND isPR = ?
		ORDER BY lastAccessed DESC`
	rows, err := db.Query(query, repoID, isPR)
	if err != nil {
		return nil, err
	}

	var entries []*DBEntry
	for rows.Next() {
		entry := DBEntry{}
		var unixTime int64
		if err := rows.Scan(&entry.Number, &unixTime, &entry.Stats.Count, &entry.IsPR, &entry.Title); err != nil {
			return nil, err
		}
		entry.Stats.LastAccessed = time.Unix(unixTime, 0)
		entries = append(entries, &entry)
	}

	return entries, nil
}

func createTables(db *sql.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS owners( 
		id INTEGER PRIMARY KEY AUTOINCREMENT, 
		gqlID TEXT NOT NULL UNIQUE,
		name TEXT NOT NULL 
	);

	CREATE TABLE IF NOT EXISTS repos(
		id INTEGER PRIMARY KEY AUTOINCREMENT, 
 		gqlID TEXT NOT NULL UNIQUE,
 		name TEXT NOT NULL,
		ownerID TEXT NOT NULL, 
		issuesLastQueried INTEGER,
		prsLastQueried INTEGER,
 		FOREIGN KEY (ownerID) REFERENCES owners(gqlID)
	);
	
	CREATE TABLE IF NOT EXISTS issues(
		id INTEGER PRIMARY KEY AUTOINCREMENT, 
		gqlID TEXT NOT NULL UNIQUE,
		title TEXT NOT NULL,
		number INTEGER NOT NULL,
		count INTEGER NOT NULL,
		lastAccessed INTEGER NOT NULL,
		repoID TEXT NOT NULL,
		isPR BOOLEAN NOT NULL DEFAULT 0,
		FOREIGN KEY (repoID) REFERENCES repo(gqlID)
	);

	CREATE INDEX IF NOT EXISTS 
	frecent ON issues(lastAccessed, count);
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
