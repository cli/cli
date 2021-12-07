package frecency

import (
	"database/sql"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/iostreams"
)

//TODO: garbage collecting

// GetFrecent, RecordAccess
type Manager struct {
	config  config.Config
	client  *http.Client
	io      *iostreams.IOStreams
	db      *sql.DB
	dataDir func() string
}

func NewManager(io *iostreams.IOStreams, cfg config.Config, client *http.Client) *Manager {
	m := &Manager{
		io:      io,
		config:  cfg,
		dataDir: config.StateDir,
		client:  client,
	}
	m.initDB()
	return m
}

func (m *Manager) getDB() (*sql.DB, error) {
	if m.db == nil {
		var err error
		dir := filepath.Join(m.dataDir(), "frecent.db")
		m.db, err = sql.Open("sqlite3", dir)
		if err != nil {
			return m.db, err
		}
	}
	// should Ping here?
	return m.db, nil
}

func (m *Manager) GetFrecentIssues(repoName string) ([]api.Issue, error) {
	var frecentIssues []api.Issue
	db, err := m.getDB()
	if err != nil {
		return frecentIssues, err
	}

	var issuesWithStats ByFrecency
	repoDetails := entryWithStats{
		RepoName: repoName,
		IsPR:     false,
	}
	issuesWithStats, err = getEntries(db, repoDetails)
	if err != nil {
		return frecentIssues, err
	}

	sort.Sort(issuesWithStats)
	for _, entry := range issuesWithStats {
		frecentIssues = append(frecentIssues, entry.Entry)
	}
	return frecentIssues, nil
}

func (m *Manager) GetFrecentPullRequests(repoName string) ([]api.PullRequest, error) {
	var frecentPRs []api.PullRequest
	db, err := m.getDB()
	if err != nil {
		return frecentPRs, err
	}

	var prsWithStats ByFrecency
	repoDetails := entryWithStats{
		RepoName: repoName,
		IsPR:     true,
	}
	prsWithStats, err = getEntries(db, repoDetails)
	if err != nil {
		return frecentPRs, err
	}

	sort.Sort(prsWithStats)
	for _, entry := range prsWithStats {
		pr := api.PullRequest{Number: entry.Entry.Number, Title: entry.Entry.Title}
		frecentPRs = append(frecentPRs, pr)
	}
	return frecentPRs, nil
}

// Records access to issue or PR at specified time
func (m *Manager) RecordAccess(repoName string, number int, timestamp time.Time) error {
	db, err := m.getDB()
	if err != nil {
		return err
	}
	repoDetails := entryWithStats{
		RepoName: repoName,
		Entry: api.Issue{
			Number: number,
		},
	}
	entry, err := getEntryByNumber(db, repoDetails)
	if err != nil {
		// if errors.Is(err, sql.ErrNoRows) {
		// 	no entry in the DB.. create a new entry?
		// }
		return err
	}

	entry.Stats.Count++
	entry.Stats.LastAccess = timestamp
	return updateEntry(db, entry)
}

// Initializes the sql database
func (m *Manager) initDB() error {
	dir := filepath.Join(m.dataDir(), "frecent.db")
	fileExists := true
	if _, err := os.Stat(dir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fileExists = false
		} else {
			return err
		}
	}
	if fileExists {
		return nil
	}

	err := os.MkdirAll(filepath.Dir(dir), 0755)
	if err != nil {
		return err
	}

	db, err := m.getDB()
	if err != nil {
		return err
	}
	return createTables(db)
}

func (m *Manager) closeDB() error {
	return m.db.Close()
}

// get issues that were created after last query time
func (m *Manager) getNewIssues(db *sql.DB, repoName string) ([]api.Issue, error) {
	db, err := m.getDB()
	if err != nil {
		return nil, err
	}

	//before the forward slash is the owner and after is the repo name
	repo := strings.Split(repoName, "/")
	fullName := ghrepo.NewWithHost(ghinstance.Default(), repo[0], repo[1])

	repoDetails := entryWithStats{RepoName: repoName}
	lastQueried, err := getLastQueried(db, repoDetails)
	newIssues, err := getIssues(m.client, fullName, lastQueried)
	if err != nil {
		return nil, err
	}

	repoDetails.Stats = countEntry{LastAccess: time.Now()}
	err = updateLastQueried(db, repoDetails)
	if err != nil {
		return nil, err
	}
	return newIssues, nil
}

// get PRs that were created after last query time
func (m *Manager) getNewPullRequests(db *sql.DB, repoName string) ([]api.PullRequest, error) {
	db, err := m.getDB()
	if err != nil {
		return nil, err
	}

	//before the forward slash is the owner and after is the repo name
	repo := strings.Split(repoName, "/")
	fullName := ghrepo.NewWithHost(ghinstance.Default(), repo[0], repo[1])

	repoDetails := entryWithStats{RepoName: repoName, IsPR: true}
	lastQueried, err := getLastQueried(db, repoDetails)
	newPRs, err := getPullRequests(m.client, fullName, lastQueried)
	if err != nil {
		return nil, err
	}

	repoDetails.Stats = countEntry{LastAccess: time.Now()}
	err = updateLastQueried(db, repoDetails)
	if err != nil {
		return nil, err
	}
	return newPRs, nil
}
