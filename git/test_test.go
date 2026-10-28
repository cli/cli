package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsolateConfigKeepsGitOperationsInTemporaryRepository(t *testing.T) {
	// Given an inherited Git directory pointing at a different repository
	IsolateConfig(t)
	ambientDir := t.TempDir()
	cmd := exec.CommandContext(t.Context(), "git", "init", "--quiet", "--bare", ambientDir)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", output)
	configPath := filepath.Join(ambientDir, "config")
	originalConfig, err := os.ReadFile(configPath)
	require.NoError(t, err)
	t.Setenv("GIT_DIR", ambientDir)
	repoDir := t.TempDir()

	// When isolation is applied before setting up and configuring a test repository
	t.Run("isolated repository", func(t *testing.T) {
		IsolateConfig(t)
		client := Client{RepoDir: repoDir}
		cmd, err := client.Command(t.Context(), "init", "--quiet")
		require.NoError(t, err)
		_, err = cmd.Output()
		require.NoError(t, err)
		require.NoError(t, client.SetRemoteResolution(t.Context(), "origin", "base"))
	})

	// Then the ambient repository is unchanged and the test repository owns the setting
	config, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Equal(t, string(originalConfig), string(config))
	config, err = os.ReadFile(filepath.Join(repoDir, ".git", "config"))
	require.NoError(t, err)
	assert.Contains(t, string(config), "gh-resolved = base")
}

func TestIsolateConfigUnsetsAndRestoresRepositoryEnvironment(t *testing.T) {
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
		t.Run(key, func(t *testing.T) {
			// Given an inherited repository path
			t.Setenv(key, "inherited")

			// When isolation is scoped to a test
			t.Run("isolated environment", func(t *testing.T) {
				IsolateConfig(t)

				// Then Git cannot see the inherited path
				_, exists := os.LookupEnv(key)
				assert.False(t, exists, "expected %s to be unset, not empty", key)
			})

			// Then cleanup restores the inherited path
			assert.Equal(t, "inherited", os.Getenv(key))
		})
	}
}
