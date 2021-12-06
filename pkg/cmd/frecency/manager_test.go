package frecency

import (
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
)

func newTestManager(dir string, io *iostreams.IOStreams) *Manager {
	m := &Manager{
		config:  config.NewBlankConfig(),
		io:      io,
		dataDir: dir,
	}
	m.initDB()
	return m
}

func testDataset() []entryWithStats {
	testTime, err := time.Parse("2006-Jan-02", "2021-Dec-03")
	if err != nil {
		log.Fatal(err)
	}
	return []entryWithStats{
		{
			Entry:    api.Issue{Number: 4827, Title: "Allow `auth status` to have reduced scope requirements"},
			Stats:    countEntry{Count: 1, LastAccess: testTime},
			RepoName: "cli/cli",
		},
		{
			Entry:    api.Issue{Number: 4567, Title: "repo create rewrite"},
			Stats:    countEntry{Count: 10, LastAccess: testTime.AddDate(0, 0, -3)},
			RepoName: "cli/cli",
		},
		{
			Entry:    api.Issue{Number: 4746, Title: "`gh browse` can't handle gist repos"},
			Stats:    countEntry{Count: 1, LastAccess: testTime.AddDate(0, 0, -1)},
			RepoName: "cli/cli",
		},
		{
			IsPR:     true,
			Entry:    api.Issue{Number: 4753, Title: "hack: frecency spike"},
			Stats:    countEntry{Count: 2, LastAccess: testTime},
			RepoName: "cli/cli",
		},
		{
			IsPR:     true,
			Entry:    api.Issue{Number: 4578, Title: "rewrite `gh repo create`"},
			Stats:    countEntry{Count: 10, LastAccess: testTime.AddDate(0, 0, -4)},
			RepoName: "cli/cli",
		},
		{
			Entry:    api.Issue{Number: 5, Title: "[Discussion] Desired features"},
			Stats:    countEntry{Count: 3, LastAccess: testTime},
			RepoName: "cli/go-gh",
		},
	}
}

func TestManager_insert_get(t *testing.T) {
	tempDir := t.TempDir()
	tempDB := filepath.Join(tempDir, "frecent.db")
	m := newTestManager(tempDB, nil)
	defer m.closeDB()
	t.Cleanup(func() { os.Remove(tempDB) })

	db, err := m.getDB()
	if err != nil {
		t.Fatal(err)
	}
	testData := testDataset()
	testTime := testData[0].Stats.LastAccess
	for _, entry := range testData {
		err := insertEntry(db, entry)
		assert.NoError(t, err)
	}

	entry := entryWithStats{
		RepoName: "cli/cli",
		IsPR:     false,
	}
	issues, err := getEntries(db, entry)
	assert.NoError(t, err)
	assert.Equal(t, 3, len(issues))
	latestIssue := issues[0]

	assert.Equal(t, latestIssue.Entry.Number, 4827)
	assert.Equal(t, latestIssue.Entry.Title, "Allow `auth status` to have reduced scope requirements")
	assert.Equal(t, latestIssue.Stats.LastAccess.UTC(), testTime.UTC())
	assert.False(t, latestIssue.IsPR)

	entry = entryWithStats{
		RepoName: "cli/cli",
		IsPR:     true,
	}
	prs, err := getEntries(db, entry)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(prs))
	latestPR := prs[0]

	assert.Equal(t, latestPR.Entry.Number, 4753)
	assert.Equal(t, latestPR.Entry.Title, "hack: frecency spike")
	assert.Equal(t, latestPR.Stats.LastAccess.UTC(), testTime.UTC())
	assert.True(t, latestPR.IsPR)

}

func TestManager_update(t *testing.T) {
	tempDir := t.TempDir()
	tempDB := filepath.Join(tempDir, "frecent.db")
	m := newTestManager(tempDB, nil)
	defer m.closeDB()
	t.Cleanup(func() { os.Remove(tempDB) })

	db, err := m.getDB()
	if err != nil {
		t.Fatal(err)
	}
	testData := testDataset()
	testTime := testData[0].Stats.LastAccess
	for _, entry := range testData {
		err := insertEntry(db, entry)
		assert.NoError(t, err)
	}

	updated := entryWithStats{
		RepoName: "cli/cli",
		Entry:    api.Issue{Number: 4827},
		Stats:    countEntry{LastAccess: testTime.AddDate(0, 0, 2), Count: 4},
	}
	err = updateEntry(db, updated)
	assert.NoError(t, err)

	gotEntry, err := getEntryByNumber(db, updated)
	assert.NoError(t, err)
	assert.Equal(t, gotEntry.Stats.Count, updated.Stats.Count)
	assert.Equal(t, gotEntry.Stats.LastAccess.UTC(), updated.Stats.LastAccess.UTC())
}
