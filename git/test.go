package git

import (
	"path/filepath"
	"testing"
)

// IsolateConfig prevents the ambient git configuration from reaching tests that shell
// out to real git.
//
// https://git-scm.com/docs/git-config#ENVIRONMENT
func IsolateConfig(t *testing.T) {
	t.Helper()

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
