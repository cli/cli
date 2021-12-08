package frecency

import (
	"database/sql"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/iostreams"
)

// NOTE: frecency treats PRs as Issues, with an extra bool flag to distinguish them

type Manager struct {
	config  config.Config
	client  *http.Client
	io      *iostreams.IOStreams
	db      *sql.DB
	dataDir func() string
}

func NewManager(io *iostreams.IOStreams) *Manager {
	m := &Manager{
		io:      io,
		dataDir: config.StateDir,
	}
	_ = m.initDB()
	return m
}

func (m *Manager) SetConfig(cfg config.Config) {
	m.config = cfg
}

func (m *Manager) SetClient(client *http.Client) {
	m.client = client
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

// Gets suggestions with frecency algorithm
// If `isPR` flag is true then suggest pull requests, else suggest issues
func (m *Manager) GetFrecent(repo ghrepo.Interface, isPR bool) ([]api.Issue, error) {
	db, err := m.getDB()
	if err != nil {
		return nil, err
	}

	// first get issues created since last query, then sort frecent
	now := time.Now()
	repoName := ghrepo.FullName(repo)
	args := entryWithStats{
		IsPR:     isPR,
		RepoName: repoName,
		Stats:    countEntry{LastAccess: now},
	}

	lastQueried, err := getLastQueried(db, args)
	if err != nil {
		return nil, err
	}

	var newEntries []api.Issue
	pruneTime, _ := time.ParseDuration("60s") // tweak this
	if time.Since(lastQueried) < pruneTime {
		// no pruning; just get new issues/PRs created since last query
		if isPR {
			if newEntries, err = getPullRequestsSince(m.client, repo, lastQueried); err != nil {
				return nil, err
			}
		} else if newEntries, err = getIssuesSince(m.client, repo, lastQueried); err != nil {
			return nil, err
		}
	} else {
		// prune stale records
		if err := m.pruneRecords(repo, isPR, 1); err != nil {
			return nil, err
		}

		// find all new issues/PRs, since most of them were pruned
		if isPR {
			if newEntries, err = getPullRequests(m.client, repo); err != nil {
				return nil, err
			}
		} else if newEntries, err = getIssuesSince(m.client, repo, time.Unix(0, 0)); err != nil {
			return nil, err
		}
	}

	err = updateLastQueried(db, args)
	if err != nil {
		return nil, err
	}

	for _, newEntry := range newEntries {
		entry := entryWithStats{
			RepoName: repoName,
			IsPR:     isPR,
			Entry:    newEntry,
			Stats:    countEntry{LastAccess: now},
		}
		if err := insertEntry(db, entry); err != nil {
			return nil, err
		}
	}

	entriesWithStats, err := getEntries(db, args)
	if err != nil {
		return nil, err
	}
	sort.Sort(ByFrecency(entriesWithStats))
	var frecentIssues []api.Issue
	for _, entry := range entriesWithStats {
		frecentIssues = append(frecentIssues, entry.Entry)
	}
	return frecentIssues, nil
}

// Records access to issue at specified time
func (m *Manager) UpdateIssue(repo ghrepo.Interface, issue *api.Issue, timestamp time.Time) error {
	db, err := m.getDB()
	if err != nil {
		return err
	}

	// search for issue by number, then update its stats
	updated := entryWithStats{
		RepoName: ghrepo.FullName(repo),
		Entry:    api.Issue{Number: issue.Number, Title: issue.Title},
		Stats:    countEntry{LastAccess: timestamp},
	}
	gotIssue, err := getEntryByNumber(db, updated)
	if err != nil {
		return err
	}
	updated.Stats.Count = gotIssue.Stats.Count + 1
	return updateEntry(db, updated)
}

// Records access to PR at specified time
func (m *Manager) UpdatePullRequest(repo ghrepo.Interface, pr *api.PullRequest, timestamp time.Time) error {
	db, err := m.getDB()
	if err != nil {
		return err
	}
	updated := entryWithStats{
		RepoName: ghrepo.FullName(repo),
		IsPR:     true,
		Entry:    api.Issue{Number: pr.Number, Title: pr.Title},
		Stats:    countEntry{LastAccess: timestamp},
	}
	gotPR, err := getEntryByNumber(db, updated)
	if err != nil {
		return err
	}
	updated.Stats.Count = gotPR.Stats.Count + 1
	return updateEntry(db, updated)
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

// strategy: drop entries in the database with few visits,
// then check if the remaining are still open
func (m *Manager) pruneRecords(repo ghrepo.Interface, isPR bool, countThreshold int) error {
	db, err := m.getDB()
	if err != nil {
		return err
	}

	args := entryWithStats{
		RepoName: ghrepo.FullName(repo),
		IsPR:     isPR,
		Stats:    countEntry{Count: countThreshold},
	}
	if err := deleteByCount(db, args); err != nil {
		return err
	}
	issues, err := getEntries(db, args)
	if err != nil {
		return err
	}

	// delete from database if no longer open
	for _, issue := range issues {
		open, err := isOpen(m.client, repo, issue)
		if err != nil {
			return err
		}
		if !open {
			if err := deleteByNumber(db, issue); err != nil {
				return err
			}
		}
	}
	return nil
}

// Deletes an issue/PR with specified number from the database
func (m *Manager) DeleteByNumber(repo ghrepo.Interface, isPR bool, number int) error {
	db, err := m.getDB()
	if err != nil {
		return err
	}
	args := entryWithStats{
		RepoName: ghrepo.FullName(repo),
		IsPR:     isPR,
		Entry:    api.Issue{Number: number},
	}
	return deleteByNumber(db, args)
}
