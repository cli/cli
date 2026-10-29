package git

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// IsolateConfig prevents ambient Git configuration and repository routing from
// reaching tests that shell out to real Git.
//
// https://git-scm.com/docs/git-config#ENVIRONMENT
func IsolateConfig(t *testing.T) {
	t.Helper()

	// Isolating global/system config does not override Git's environment variables.
	// These can redirect repository, config, index, object, or shallow-file access
	// outside the test directory, or change discovery and initialization defaults
	// (templates, index version, object hash, and ref format). Unset them so tests
	// use their own repositories and Git's defaults, not the caller's environment.
	// https://git-scm.com/docs/git#_environment_variables
	for _, variable := range []string{
		"GIT_DIR",
		"GIT_WORK_TREE",
		"GIT_COMMON_DIR",
		"GIT_CONFIG",
		"GIT_INDEX_FILE",
		"GIT_INDEX_VERSION",
		"GIT_OBJECT_DIRECTORY",
		"GIT_ALTERNATE_OBJECT_DIRECTORIES",
		"GIT_NAMESPACE",
		"GIT_SHALLOW_FILE",
		"GIT_CEILING_DIRECTORIES",
		"GIT_DISCOVERY_ACROSS_FILESYSTEM",
		"GIT_TEMPLATE_DIR",
		"GIT_DEFAULT_HASH",
		"GIT_DEFAULT_REF_FORMAT",
	} {
		unsetEnv(t, variable)
	}

	// Point the global config at a test-owned file that disables the caller's
	// global ignore and attributes rules, and ignore the system config.
	configDir := t.TempDir()
	emptyRulesFile := filepath.Join(configDir, "empty-rules")
	if err := os.WriteFile(emptyRulesFile, nil, 0600); err != nil {
		t.Fatalf("failed to create empty Git rules file: %v", err)
	}
	globalConfig := filepath.Join(configDir, ".gitconfig")
	config := fmt.Sprintf("[core]\n\texcludesFile = %[1]q\n\tattributesFile = %[1]q\n", filepath.ToSlash(emptyRulesFile))
	if err := os.WriteFile(globalConfig, []byte(config), 0600); err != nil {
		t.Fatalf("failed to create global Git config: %v", err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", globalConfig)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "true")

	// Config from these vars is command line scope, which outranks the global and
	// system files, so redirecting those files alone leaves it in place. Tools that
	// wrap git inject config this way, and an inherited safe.bareRepository=explicit
	// makes git refuse to open a bare repository at all.
	t.Setenv("GIT_CONFIG_COUNT", "")
	t.Setenv("GIT_CONFIG_PARAMETERS", "")
}

func unsetEnv(t *testing.T, name string) {
	t.Helper()

	value, found := os.LookupEnv(name)
	if err := os.Unsetenv(name); err != nil {
		t.Fatalf("failed to unset %s: %v", name, err)
	}
	t.Cleanup(func() {
		var err error
		if found {
			err = os.Setenv(name, value)
		} else {
			err = os.Unsetenv(name)
		}
		if err != nil {
			t.Errorf("failed to restore %s: %v", name, err)
		}
	})
}
