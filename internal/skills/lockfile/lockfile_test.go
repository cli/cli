package lockfile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/cli/cli/v2/internal/flock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestHome redirects HOME to a temp dir and returns the expected lockfile path.
func setupTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return filepath.Join(home, agentsDir, lockFile)
}

func TestRecordInstall(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T)
		host      string
		skill     string
		owner     string
		repo      string
		skillPath string
		treeSHA   string
		pinnedRef string
		wantErr   bool
		verify    func(t *testing.T, lockPath string)
	}{
		{
			name:      "fresh install creates lockfile",
			host:      "github.com",
			skill:     "code-review",
			owner:     "monalisa",
			repo:      "octocat-skills",
			skillPath: "skills/code-review/SKILL.md",
			treeSHA:   "abc123",
			verify: func(t *testing.T, lockPath string) {
				t.Helper()
				f := readTestLockfile(t, lockPath)
				require.Contains(t, f.Skills, "code-review")
				e := f.Skills["code-review"]
				assert.Equal(t, "monalisa/octocat-skills", e.Source)
				assert.Equal(t, "github", e.SourceType)
				assert.Equal(t, "https://github.com/monalisa/octocat-skills.git", e.SourceURL)
				assert.Equal(t, "skills/code-review/SKILL.md", e.SkillPath)
				assert.Equal(t, "abc123", e.SkillFolderHash)
				assert.NotEmpty(t, e.InstalledAt)
				assert.NotEmpty(t, e.UpdatedAt)
				assert.Empty(t, e.PinnedRef)
			},
		},
		{
			name:      "tenancy host uses correct URL",
			host:      "mycompany.ghe.com",
			skill:     "code-review",
			owner:     "monalisa",
			repo:      "octocat-skills",
			skillPath: "skills/code-review/SKILL.md",
			treeSHA:   "abc123",
			verify: func(t *testing.T, lockPath string) {
				t.Helper()
				f := readTestLockfile(t, lockPath)
				require.Contains(t, f.Skills, "code-review")
				e := f.Skills["code-review"]
				assert.Equal(t, "https://mycompany.ghe.com/monalisa/octocat-skills.git", e.SourceURL)
			},
		},
		{
			name:      "install with pinned ref",
			host:      "github.com",
			skill:     "pr-summary",
			owner:     "hubot",
			repo:      "skills-repo",
			skillPath: "skills/pr-summary/SKILL.md",
			treeSHA:   "def456",
			pinnedRef: "v1.0.0",
			verify: func(t *testing.T, lockPath string) {
				t.Helper()
				f := readTestLockfile(t, lockPath)
				assert.Equal(t, "v1.0.0", f.Skills["pr-summary"].PinnedRef)
			},
		},
		{
			name: "multiple skills coexist",
			setup: func(t *testing.T) {
				t.Helper()
				require.NoError(t, RecordInstall("github.com", "code-review", "monalisa", "octocat-skills", "skills/code-review/SKILL.md", "sha1", ""))
			},
			host:      "github.com",
			skill:     "issue-triage",
			owner:     "monalisa",
			repo:      "octocat-skills",
			skillPath: "skills/issue-triage/SKILL.md",
			treeSHA:   "sha2",
			verify: func(t *testing.T, lockPath string) {
				t.Helper()
				f := readTestLockfile(t, lockPath)
				assert.Contains(t, f.Skills, "code-review")
				assert.Contains(t, f.Skills, "issue-triage")
			},
		},
		{
			name: "returns error when lock cannot be acquired",
			setup: func(t *testing.T) {
				t.Helper()
				origAttempts := lockAttempts
				origDelay := lockAttemptDelay
				lockAttempts = 1
				lockAttemptDelay = 0
				t.Cleanup(func() {
					lockAttempts = origAttempts
					lockAttemptDelay = origDelay
				})
				// Hold a real flock so acquireFLock fails.
				lockPath, err := lockfilePath()
				require.NoError(t, err)
				require.NoError(t, os.MkdirAll(filepath.Dir(lockPath), 0o755))
				_, unlock, err := flock.TryLock(lockPath)
				require.NoError(t, err)
				t.Cleanup(unlock)
			},
			host:      "github.com",
			skill:     "code-review",
			owner:     "monalisa",
			repo:      "octocat-skills",
			skillPath: "skills/code-review/SKILL.md",
			treeSHA:   "abc123",
			wantErr:   true,
		},
		{
			name: "recovers from corrupt lockfile",
			setup: func(t *testing.T) {
				t.Helper()
				lockPath, err := lockfilePath()
				require.NoError(t, err)
				require.NoError(t, os.MkdirAll(filepath.Dir(lockPath), 0o755))
				require.NoError(t, os.WriteFile(lockPath, []byte("{invalid json"), 0o644))
			},
			host:      "github.com",
			skill:     "code-review",
			owner:     "monalisa",
			repo:      "octocat-skills",
			skillPath: "skills/code-review/SKILL.md",
			treeSHA:   "abc123",
			verify: func(t *testing.T, lockPath string) {
				t.Helper()
				f := readTestLockfile(t, lockPath)
				assert.Equal(t, lockVersion, f.Version)
				require.Contains(t, f.Skills, "code-review")
			},
		},
		{
			name: "recovers from wrong version lockfile",
			setup: func(t *testing.T) {
				t.Helper()
				lockPath, err := lockfilePath()
				require.NoError(t, err)
				require.NoError(t, os.MkdirAll(filepath.Dir(lockPath), 0o755))
				data, _ := json.Marshal(file{Version: 999, Skills: map[string]entry{"old-skill": {}}})
				require.NoError(t, os.WriteFile(lockPath, data, 0o644))
			},
			host:      "github.com",
			skill:     "code-review",
			owner:     "monalisa",
			repo:      "octocat-skills",
			skillPath: "skills/code-review/SKILL.md",
			treeSHA:   "abc123",
			verify: func(t *testing.T, lockPath string) {
				t.Helper()
				f := readTestLockfile(t, lockPath)
				assert.Equal(t, lockVersion, f.Version)
				require.Contains(t, f.Skills, "code-review")
				assert.NotContains(t, f.Skills, "old-skill", "wrong-version data should be discarded")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lockPath := setupTestHome(t)
			if tt.setup != nil {
				tt.setup(t)
			}

			err := RecordInstall(tt.host, tt.skill, tt.owner, tt.repo, tt.skillPath, tt.treeSHA, tt.pinnedRef)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			tt.verify(t, lockPath)
		})
	}

	// This case lives outside the table because it needs to read the lockfile
	// between two RecordInstall calls to capture the first InstalledAt value.
	t.Run("update preserves InstalledAt and updates treeSHA", func(t *testing.T) {
		lockPath := setupTestHome(t)

		require.NoError(t, RecordInstall("github.com", "code-review", "monalisa", "octocat-skills", "skills/code-review/SKILL.md", "old-sha", ""))
		firstInstalledAt := readTestLockfile(t, lockPath).Skills["code-review"].InstalledAt

		require.NoError(t, RecordInstall("github.com", "code-review", "monalisa", "octocat-skills", "skills/code-review/SKILL.md", "new-sha", ""))
		entry := readTestLockfile(t, lockPath).Skills["code-review"]

		assert.Equal(t, "new-sha", entry.SkillFolderHash, "treeSHA should be updated")
		assert.Equal(t, firstInstalledAt, entry.InstalledAt, "InstalledAt should be preserved from first install")
	})
}

// readTestLockfile is a test helper that reads and parses the lockfile from disk.
func readTestLockfile(t *testing.T, path string) *file {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err, "lockfile should exist at %s", path)
	var f file
	require.NoError(t, json.Unmarshal(data, &f))
	return &f
}
