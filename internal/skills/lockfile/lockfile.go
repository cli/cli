package lockfile

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/cli/cli/v2/internal/flock"
	"github.com/cli/cli/v2/internal/ghinstance"
)

const (
	// lockVersion must match Vercel's CURRENT_LOCK_VERSION for interop.
	lockVersion = 3
	agentsDir   = ".agents"
	lockFile    = ".skill-lock.json"
)

// entry represents a single installed skill in the lock file.
type entry struct {
	Source          string `json:"source"`
	SourceType      string `json:"sourceType"`
	SourceURL       string `json:"sourceUrl"`
	SkillPath       string `json:"skillPath,omitempty"`
	SkillFolderHash string `json:"skillFolderHash"`
	InstalledAt     string `json:"installedAt"`
	UpdatedAt       string `json:"updatedAt"`
	PinnedRef       string `json:"pinnedRef,omitempty"`
}

// file is the top-level structure of .skill-lock.json.
type file struct {
	Version   int              `json:"version"`
	Skills    map[string]entry `json:"skills"`
	Dismissed map[string]bool  `json:"dismissed,omitempty"`
}

// lockfilePath returns the absolute path to the lock file.
func lockfilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, agentsDir, lockFile), nil
}

// readFrom loads the lock file from an open file handle.
// Returns an empty file if the content is empty, corrupt, or incompatible.
func readFrom(f *os.File) (*file, error) {
	if _, err := f.Seek(0, 0); err != nil {
		return nil, fmt.Errorf("could not seek lock file: %w", err)
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("could not read lock file: %w", err)
	}
	if len(data) == 0 {
		return newFile(), nil
	}

	var lf file
	if err := json.Unmarshal(data, &lf); err != nil {
		return newFile(), nil //nolint:nilerr // graceful: corrupt file means fresh state
	}

	if lf.Version != lockVersion || lf.Skills == nil {
		return newFile(), nil
	}

	return &lf, nil
}

// writeTo persists the lock file through an open file handle.
func writeTo(f *os.File, lf *file) error {
	data, err := json.MarshalIndent(lf, "", "  ")
	if err != nil {
		return err
	}

	if _, err := f.Seek(0, 0); err != nil {
		return err
	}
	if err := f.Truncate(0); err != nil {
		return err
	}
	_, err = f.Write(data)
	return err
}

// RecordInstall adds or updates a skill entry in the lock file.
// It uses a file-based lock to prevent concurrent read-modify-write races
// when multiple install processes run simultaneously.
func RecordInstall(host, skillName, owner, repo, skillPath, treeSHA, pinnedRef string) error {
	lockPath, err := lockfilePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return fmt.Errorf("could not create lock directory: %w", err)
	}

	lockedFile, unlock, err := acquireFLock()
	if err != nil {
		return err
	}
	defer unlock()

	f, err := readFrom(lockedFile)
	if err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)

	existing, exists := f.Skills[skillName]
	installedAt := now
	if exists {
		installedAt = existing.InstalledAt
	}

	f.Skills[skillName] = entry{
		Source:          owner + "/" + repo,
		SourceType:      "github",
		SourceURL:       ghinstance.HostPrefix(host) + owner + "/" + repo + ".git",
		SkillPath:       skillPath,
		SkillFolderHash: treeSHA,
		InstalledAt:     installedAt,
		UpdatedAt:       now,
		PinnedRef:       pinnedRef,
	}

	return writeTo(lockedFile, f)
}

func newFile() *file {
	return &file{
		Version: lockVersion,
		Skills:  make(map[string]entry),
	}
}

var (
	lockAttempts     = 30
	lockAttemptDelay = 100 * time.Millisecond
)

// acquireFLock attempts to acquire an exclusive file lock to serialize concurrent access.
// Returns the locked file handle and an unlock function, or an error if the lock
// cannot be acquired. The caller should read/write through the returned file to
// avoid Windows mandatory lock conflicts.
func acquireFLock() (f *os.File, unlock func(), err error) {
	lockPath, err := lockfilePath()
	if err != nil {
		return nil, nil, fmt.Errorf("could not determine lock path: %w", err)
	}

	var lastErr error
	for attempt := range lockAttempts {
		f, unlock, err := flock.TryLock(lockPath)
		if err == nil {
			return f, unlock, nil
		}
		lastErr = err

		if !errors.Is(err, flock.ErrLocked) {
			return nil, nil, err
		}
		if attempt < lockAttempts-1 {
			time.Sleep(lockAttemptDelay)
		}
	}

	return nil, nil, fmt.Errorf("could not acquire lock after %d attempts: %w", lockAttempts, lastErr)
}
