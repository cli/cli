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

//TODO: pruning stale records

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

// Suggests issues from frecency stats
func (m *Manager) GetFrecentIssues(repo ghrepo.Interface) ([]api.Issue, error) {
	var frecentIssues []api.Issue
	db, err := m.getDB()
	if err != nil {
		return frecentIssues, err
	}

	// first get issues created since last query, then sort frecent
	repoName := ghrepo.FullName(repo)
	args := entryWithStats{
		RepoName: repoName,
		Stats:    countEntry{LastAccess: time.Now()},
	}

	lastQueried, err := getLastQueried(db, args)
	if err != nil {
		return nil, err
	}
	newIssues, err := getIssuesSince(m.client, repo, lastQueried)

	if err != nil {
		return nil, err
	}

	for _, newIssue := range newIssues {
		entry := entryWithStats{
			RepoName: repoName,
			Entry:    newIssue,
			Stats:    countEntry{LastAccess: time.Now()},
		}

		if err := insertEntry(db, entry); err != nil {
			return nil, err
		}
	}

	// update query timestamp
	err = updateLastQueried(db, args)
	if err != nil {
		return nil, err
	}

	issuesWithStats, err := getEntries(db, args)
	if err != nil {
		return nil, err
	}
	sort.Sort(ByFrecency(issuesWithStats))
	for _, issueWithStats := range issuesWithStats {
		frecentIssues = append(frecentIssues, issueWithStats.Entry)
	}
	return frecentIssues, nil
}

func (m *Manager) GetFrecentPullRequests(repo ghrepo.Interface) ([]api.PullRequest, error) {
	var frecentPRs []api.PullRequest
	db, err := m.getDB()
	if err != nil {
		return nil, err
	}

	repoName := ghrepo.FullName(repo)
	args := entryWithStats{
		RepoName: repoName,
		IsPR:     true,
		Stats:    countEntry{LastAccess: time.Now()},
	}
	lastQueried, err := getLastQueried(db, args)
	newPRs, err := getPullRequestsSince(m.client, repo, lastQueried)
	if err != nil {
		return nil, err
	}

	for _, newPR := range newPRs {
		entry := entryWithStats{
			RepoName: repoName,
			Entry:    newPR,
			IsPR:     true,
			Stats:    countEntry{LastAccess: time.Now()},
		}
		if err := insertEntry(db, entry); err != nil {
			return nil, err
		}
	}
	err = updateLastQueried(db, args)
	if err != nil {
		return nil, err
	}

	prsWithStats, err := getEntries(db, args)
	if err != nil {
		return nil, err
	}
	sort.Sort(ByFrecency(prsWithStats))
	for _, entry := range prsWithStats {
		pr := api.PullRequest{Number: entry.Entry.Number, Title: entry.Entry.Title}
		frecentPRs = append(frecentPRs, pr)
	}
	return frecentPRs, nil
}

// Records access to issue or PR at specified time
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
		if errors.Is(err, sql.ErrNoRows) {
			// no entry in the DB, insert it
			updated.Stats.Count = 1
			if insertErr := insertEntry(db, updated); err != nil {
				return insertErr
			}
		}
		return err
	}

	updated.Stats.Count = gotIssue.Stats.Count + 1
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

	args := entryWithStats{RepoName: ghrepo.FullName(repo), IsPR: isPR}
	if err := deleteByCount(db, args); err != nil {
		return err
	}
	//TODO: check if still open
	return nil
}
