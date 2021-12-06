package frecency

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/iostreams"
)

// GetFrecent, RecordAccess
type Manager struct {
	config  config.Config
	io      *iostreams.IOStreams
	db      *sql.DB
	dataDir string
}

func NewManager(io *iostreams.IOStreams, cfg config.Config) *Manager {
	m := &Manager{
		io:      io,
		config:  cfg,
		dataDir: filepath.Join(config.StateDir(), "frecent.db"),
	}
	m.initDB()
	return m
}

func (m *Manager) getDB() (*sql.DB, error) {
	if m.db == nil {
		db, err := sql.Open("sqlite3", m.dataDir)
		m.db = db
		if err != nil {
			return m.db, err
		}
	}
	return m.db, nil
}

func (m *Manager) GetFrecentIssues(repoName string) ([]api.Issue, error) {
	var frecentIssues []api.Issue
	db, err := m.getDB()
	if err != nil {
		return frecentIssues, err
	}

	var issuesWithStats ByFrecency
	issuesWithStats, err = getEntries(db, repoName, false)
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
	prsWithStats, err = getEntries(db, repoName, true)
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
	entry, err := getEntryByNumber(db, repoName, number)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// not in DB... what to do
		}
		return err
	}

	entry.Stats.Count++
	entry.Stats.LastAccess = timestamp
	return updateEntry(db, repoName, entry)
}

// Initializes the sql database
func (m *Manager) initDB() error {
	dir := m.dataDir
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

func (m *Manager) closeDB(db *sql.DB) error {
	return db.Close()
}

type ByFrecency []entryWithStats

func (f ByFrecency) Len() int {
	return len(f)
}
func (f ByFrecency) Swap(i, j int) {
	f[i], f[j] = f[j], f[i]
}
func (f ByFrecency) Less(i, j int) bool {
	iScore := f[i].Stats.Score()
	jScore := f[j].Stats.Score()
	if iScore == jScore {
		return f[i].Stats.LastAccess.After(f[j].Stats.LastAccess)
	}
	return iScore > jScore
}

func (c countEntry) Score() int {
	if c.Count == 0 {
		return 0
	}
	duration := time.Since(c.LastAccess)
	recencyScore := 10
	if duration < 1*time.Hour {
		recencyScore = 100
	} else if duration < 6*time.Hour {
		recencyScore = 80
	} else if duration < 24*time.Hour {
		recencyScore = 60
	} else if duration < 3*24*time.Hour {
		recencyScore = 40
	} else if duration < 7*24*time.Hour {
		recencyScore = 20
	}
	return recencyScore * c.Count
}

//func SelectFrecent(c *http.Client, repo ghrepo.Interface) (string, error) {
//	client := api.NewCachedClient(c, time.Hour*6)
//
//	issues, err := getIssues(client, repo)
//	if err != nil {
//		return "", err
//	}
//
//	frecent, err := getFrecentEntry(defaultFrecentPath())
//	if err != nil {
//		return "", err
//	}
//
//	choices := sortByFrecent(issues, frecent.Issues)
//
//	choice := ""
//	err = prompt.SurveyAskOne(&survey.Select{
//		Message: "Which issue?",
//		Options: choices,
//	}, &choice)
//	if err != nil {
//		return "", err
//	}
//
//	return choice, nil
//}
