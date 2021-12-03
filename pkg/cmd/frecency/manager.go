package frecency

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/iostreams"
)

/** Manager:
	- garbage collecting the frecency file
		- remove closed issues, prs, deleted repos, etc.
		- maybe only keep a # of issues, prs, and repos stored in the yaml file. (20?)
		- will be fairly expensive, how to schedule?

- modifying the gh __ view commands to show suggestions
- in pkg/cmd/frecency  --- add some configuration commands?
	- `gh frecency clear` etc.

Issues to consider:
- "cache invalidation": renamed repo, issue title/status, etc.
*/

// GetFrecent, RecordAccess
type Manager struct {
	config config.Config
	io     *iostreams.IOStreams
	db     func() (*sql.DB, error) // getDB
}

func NewManager(io *iostreams.IOStreams, cfg config.Config) *Manager {
	return &Manager{
		io:     io,
		config: cfg,
		db:     OpenDB,
	}
}

func OpenDB() (*sql.DB, error) {
	dbPath := filepath.Join(config.StateDir(), "frecency.db")
	fileExists := true
	if _, err := os.Stat(dbPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fileExists = false
		} else {
			return nil, err
		}
	}
	if !fileExists {
		err := os.MkdirAll(filepath.Dir(dbPath), 0755)
		if err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}
	err = createTables(db)
	if err != nil {
		return nil, err
	}
	return db, nil
}

func (m *Manager) closeDB(db *sql.DB) error {
	return db.Close()
}

func (m *Manager) GetFrecentIssue(db *sql.DB, repoName string, isPr bool) ([]entryWithStats, error) {
	return getEntries(db, repoName, isPr)
}

// func (m *Manager) RecordAccess() {

// }

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

// sorting
// type IDWithStats struct {
// 	Identifier
// 	CountEntry
// }

// type ByFrecency []entryWithStats

// func (f ByFrecency) Len() int {
// 	return len(f)
// }
// func (f ByFrecency) Swap(i, j int) {
// 	f[i], f[j] = f[j], f[i]
// }
// func (f ByFrecency) Less(i, j int) bool {
// 	iScore := f[i].countEntry.Score()
// 	jScore := f[j].countEntry.Score()
// 	if iScore == jScore {
// 		return f[i].LastAccess.After(f[j].LastAccess)
// 	}
// 	return iScore > jScore
// }

// func (c CountEntry) Score() int {
// 	if c.Count == 0 {
// 		return 0
// 	}
// 	duration := time.Since(c.Last)
// 	recencyScore := 10
// 	if duration < 1*time.Hour {
// 		recencyScore = 100
// 	} else if duration < 6*time.Hour {
// 		recencyScore = 80
// 	} else if duration < 24*time.Hour {
// 		recencyScore = 60
// 	} else if duration < 3*24*time.Hour {
// 		recencyScore = 40
// 	} else if duration < 7*24*time.Hour {
// 		recencyScore = 20
// 	}

// 	return c.Count * recencyScore
// }

//func sortByFrecent([]entryWithStats, frecent map[int]*CountEntry) []string {
//	withStats := []entryWithStats{}
//	for _, i := range identifiers {
//		entry, ok := frecent[i.Number]
//		if !ok {
//			entry = &CountEntry{}
//		}
//		withStats = append(withStats, IDWithStats{
//			Identifier: i,
//			CountEntry: *entry,
//		})
//	}
//	sort.Sort(ByLastAccess(withStats))
//
//	// why do this?
//	previousId := withStats[0]
//	withStats = withStats[1:]
//	sort.Stable(ByFrecency(withStats))
//	choices := []string{fmt.Sprintf("%d", previousId.Number)}
//	for _, ws := range withStats {
//		choices = append(choices, fmt.Sprintf("%d", ws.Number))
//	}
//	return choices
//}
