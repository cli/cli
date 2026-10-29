package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsolateConfigPreventsAmbientGitDirFromRoutingRepositoryCommands(t *testing.T) {
	// Given isolated sentinel repositories and hostile repository routing
	IsolateConfig(t)
	ambientCommonDir := filepath.Join(t.TempDir(), "common.git")
	cmd := exec.CommandContext(t.Context(), "git", "init", "--bare", "--quiet", ambientCommonDir)
	output, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git init --bare failed:\n%s", output)
	cmd = exec.CommandContext(t.Context(), "git", "--git-dir", ambientCommonDir, "config", "credential.helper", "owned-sentinel")
	output, err = cmd.CombinedOutput()
	require.NoErrorf(t, err, "git config failed:\n%s", output)
	ambientGitDir := filepath.Join(t.TempDir(), "ambient.git")
	cmd = exec.CommandContext(t.Context(), "git", "init", "--bare", "--quiet", ambientGitDir)
	output, err = cmd.CombinedOutput()
	require.NoErrorf(t, err, "git init --bare failed:\n%s", output)
	cmd = exec.CommandContext(t.Context(), "git", "--git-dir", ambientGitDir, "config", "credential.helper", "review-sentinel")
	output, err = cmd.CombinedOutput()
	require.NoErrorf(t, err, "git config failed:\n%s", output)
	t.Setenv("GIT_DIR", ambientGitDir)
	t.Setenv("GIT_COMMON_DIR", ambientCommonDir)
	repoDir := t.TempDir()
	client := Client{RepoDir: repoDir}

	// When Git is isolated and the client initializes its configured directory
	IsolateConfig(t)
	gitCmd, err := client.Command(t.Context(), "init", "--quiet", "--initial-branch=trunk")
	require.NoError(t, err)
	_, err = gitCmd.Output()
	require.NoError(t, err)
	gitCmd, err = client.Command(t.Context(), "config", "credential.helper", "test")
	require.NoError(t, err)
	_, err = gitCmd.Output()
	require.NoError(t, err)

	// Then the repository is created locally without changing the ambient repository
	_, err = os.Stat(filepath.Join(repoDir, ".git"))
	require.NoError(t, err)
	cmd = exec.CommandContext(t.Context(), "git", "--git-dir", ambientGitDir, "config", "credential.helper")
	output, err = cmd.Output()
	require.NoError(t, err)
	assert.Equal(t, "review-sentinel\n", string(output))
	cmd = exec.CommandContext(t.Context(), "git", "--git-dir", ambientCommonDir, "config", "credential.helper")
	output, err = cmd.Output()
	require.NoError(t, err)
	assert.Equal(t, "owned-sentinel\n", string(output))
}

func TestIsolateConfigPreventsAmbientGitConfigFromRoutingConfigWrites(t *testing.T) {
	// Given an independent config file selected by the ambient environment
	ambientConfig := filepath.Join(t.TempDir(), "ambient.gitconfig")
	original := []byte("[review]\n\tmarker = original\n")
	require.NoError(t, os.WriteFile(ambientConfig, original, 0600))
	t.Setenv("GIT_CONFIG", ambientConfig)
	repoDir := t.TempDir()
	client := Client{RepoDir: repoDir}

	// When Git is isolated and the client initializes and configures its repository
	IsolateConfig(t)
	gitCmd, err := client.Command(t.Context(), "init", "--quiet", "--initial-branch=trunk")
	require.NoError(t, err)
	_, err = gitCmd.Output()
	require.NoError(t, err)
	gitCmd, err = client.Command(t.Context(), "config", "credential.helper", "test")
	require.NoError(t, err)
	_, err = gitCmd.Output()
	require.NoError(t, err)
	helper, err := client.Config(t.Context(), "credential.helper")
	require.NoError(t, err)

	// Then reads and writes stay local while the independent file is unchanged
	assert.Equal(t, "test", helper)
	content, err := os.ReadFile(ambientConfig)
	require.NoError(t, err)
	assert.Equal(t, original, content)
}

func TestIsolateConfigPreventsGlobalIgnoreFromChangingClientBehavior(t *testing.T) {
	// Given a global ignore rule for a path that the repository does not ignore
	xdgConfigHome := t.TempDir()
	gitConfigDir := filepath.Join(xdgConfigHome, "git")
	require.NoError(t, os.MkdirAll(gitConfigDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(gitConfigDir, "ignore"), []byte(".github/skills\n"), 0600))
	t.Setenv("XDG_CONFIG_HOME", xdgConfigHome)
	repoDir := t.TempDir()
	client := Client{RepoDir: repoDir}

	// When Git is isolated and the path is checked in a test-owned repository
	IsolateConfig(t)
	gitCmd, err := client.Command(t.Context(), "init", "--quiet", "--initial-branch=trunk")
	require.NoError(t, err)
	_, err = gitCmd.Output()
	require.NoError(t, err)
	ignored, err := client.IsIgnored(t.Context(), ".github/skills")

	// Then the caller's global ignore rule does not affect the result
	require.NoError(t, err)
	assert.False(t, ignored, "expected the test-owned repository not to inherit the global ignore rule")
}

func TestIsolateConfigPreventsGlobalAttributesFromChangingFileEncoding(t *testing.T) {
	// Given a global attributes rule requiring UTF-16 for text files
	xdgConfigHome := t.TempDir()
	gitConfigDir := filepath.Join(xdgConfigHome, "git")
	require.NoError(t, os.MkdirAll(gitConfigDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(gitConfigDir, "attributes"), []byte("*.txt working-tree-encoding=UTF-16\n"), 0600))
	t.Setenv("XDG_CONFIG_HOME", xdgConfigHome)

	// When Git is isolated and a plain-text file is staged
	repo := newTestRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repo.dir, "tracked.txt"), []byte("tracked\n"), 0600))
	repo.run(t, "add", "tracked.txt")

	// Then the file is stored without the caller's encoding rule
	assert.Equal(t, "tracked", repo.run(t, "show", ":tracked.txt"))
}

func TestIsolateConfigClearsRepositoryRoutingEnvironment(t *testing.T) {
	// Given ambient environment that can redirect Git repository state
	variables := []string{
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
	}
	for _, variable := range variables {
		t.Setenv(variable, "ambient")
	}

	// When Git configuration is isolated
	IsolateConfig(t)

	// Then repository routing and initialization variables are removed
	for _, variable := range variables {
		_, found := os.LookupEnv(variable)
		assert.Falsef(t, found, "expected %s to be unset", variable)
	}
}
