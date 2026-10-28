package git

import (
	"os"
	"path/filepath"
	"testing"
)

// IsolateConfig prevents ambient Git configuration and repository paths from
// reaching tests that shell out to real Git.
//
// https://git-scm.com/docs/git-config#ENVIRONMENT
func IsolateConfig(t *testing.T) {
	t.Helper()

	// Repository paths override Git's working directory, including -C. Unset them
	// rather than setting empty values, which Git can interpret as invalid paths.
	for _, key := range []string{
		"GIT_DIR",
		"GIT_COMMON_DIR",
		"GIT_WORK_TREE",
		"GIT_INDEX_FILE",
		"GIT_OBJECT_DIRECTORY",
		"GIT_ALTERNATE_OBJECT_DIRECTORIES",
		"GIT_SHALLOW_FILE",
		"GIT_GRAFT_FILE",
		"GIT_CONFIG",
	} {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}

	// Point the global config at an empty file and ignore the system one.
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), ".gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "true")

	// Config from these vars is command line scope, which outranks the global and
	// system files, so redirecting those files alone leaves it in place. Tools that
	// wrap git inject config this way, and an inherited safe.bareRepository=explicit
	// makes git refuse to open a bare repository at all.
	t.Setenv("GIT_CONFIG_COUNT", "")
	t.Setenv("GIT_CONFIG_PARAMETERS", "")
}
