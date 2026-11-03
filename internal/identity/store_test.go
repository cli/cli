package identity

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func isolatedStore(t *testing.T) *Store {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("persistent identities are not supported on Windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("GH_CONFIG_DIR", filepath.Join(home, "gh"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, ".gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, key := range []string{"GH_TOKEN", "GITHUB_TOKEN", "GH_ENTERPRISE_TOKEN", "GITHUB_ENTERPRISE_TOKEN", "GIT_CONFIG_COUNT", "GIT_CONFIG_PARAMETERS", "GIT_DIR", "GIT_WORK_TREE", "GIT_SSH_COMMAND", "GIT_SSH", "GIT_SSH_VARIANT", "GH_IDENTITY_SSH_COMMAND", "GH_IDENTITY_SSH_VARIANT"} {
		t.Setenv(key, "")
		require.NoError(t, os.Unsetenv(key))
	}
	s, err := Open()
	require.NoError(t, err)
	return s
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)
	return string(out)
}

func TestIdentityBindingRequiresRepository(t *testing.T) {
	// Given an identity and an ordinary directory outside any repository
	s := isolatedStore(t)
	dir := t.TempDir()
	require.NoError(t, s.Save(Identity{Name: "work", Host: "github.com", Account: "alice"}, false))

	// When attempting to apply a repository choice outside a checkout
	err := s.Bind(Binding{Identity: "work", Directory: dir}, "/path/to/gh")

	// Then no broader directory choice or global integration is silently installed
	require.ErrorContains(t, err, "require a repository checkout")
	assert.NoFileExists(t, s.authConfigPath())
}

func TestBasicSSHRefusesCustomConnectionCommand(t *testing.T) {
	// Given a global SSH command that basic alias routing cannot safely preserve
	s := isolatedStore(t)
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "config", "--global", "core.sshCommand", "ssh -F company.conf")
	require.NoError(t, s.Save(Identity{Name: "work", Host: "github.com", Account: "alice"}, false))

	// When applying the identity
	err := s.Bind(Binding{Identity: "work", Directory: dir}, "/path/to/gh")

	// Then setup refuses explicitly and preserves the user's connection settings
	require.ErrorContains(t, err, "custom core.sshCommand is not supported")
	assert.Equal(t, "ssh -F company.conf\n", runGit(t, dir, "config", "--global", "core.sshCommand"))
	assert.NoFileExists(t, s.authConfigPath())
}

func TestRepositoryUsesLiveIdentity(t *testing.T) {
	// Given an identity referencing an account and SSH route
	s := isolatedStore(t)
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	id := Identity{Name: "work", Host: "github.com", Account: "alice", SSHHost: "github-work"}
	require.NoError(t, s.Save(id, false))

	// When that identity is applied to a repository and subsequently edited
	require.NoError(t, s.Bind(Binding{Identity: "work", Directory: repo}, "/path/to/gh"))
	id.Account = "bob"
	require.NoError(t, s.Save(id, true))
	got, err := s.Resolve("github.com", "", repo)

	// Then both gh selection and Git see the edited shared identity
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "bob", got.Account)
	assert.Equal(t, "bob\n", runGit(t, repo, "config", "credential.https://github.com.username"))
	assert.Equal(t, "work\n", runGit(t, repo, "config", "gh-identity.github.com.name"))
	contents, err := os.ReadFile(s.IdentityPath("work"))
	require.NoError(t, err)
	assert.NotContains(t, string(contents), "oauth_token")
}

func TestBindingRefusesConflictingRepositorySSHCommand(t *testing.T) {
	// Given a repository with an explicitly configured SSH command
	s := isolatedStore(t)
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	runGit(t, repo, "config", "core.sshCommand", "ssh -i another-key")
	require.NoError(t, s.Save(Identity{Name: "work", Host: "github.com", Account: "alice", SSHHost: "github-work"}, false))

	// When applying an identity that would be bypassed by that command
	err := s.Bind(Binding{Identity: "work", Directory: repo}, "/path/to/gh")

	// Then no binding is recorded and the conflict is explicit
	require.ErrorContains(t, err, "core.sshCommand")
	bindings, err := s.Bindings()
	require.NoError(t, err)
	assert.Empty(t, bindings)
}

func TestBindingSSHRepositoryRequiresRoute(t *testing.T) {
	// Given an SSH repository and an identity without an SSH route
	s := isolatedStore(t)
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	runGit(t, repo, "remote", "add", "origin", "git@github.com:company/project.git")
	require.NoError(t, s.Save(Identity{Name: "work", Host: "github.com", Account: "alice"}, false))

	// When applying that identity to the repository
	err := s.Bind(Binding{Identity: "work", Directory: repo}, "/path/to/gh")

	// Then setup fails with guidance rather than claiming SSH is configured
	require.ErrorContains(t, err, "has no SSH route")
}

func TestDirectoryChoiceFromSubdirectoryStartsAtRepositoryRoot(t *testing.T) {
	// Given an identity applied from a subdirectory of an existing repository
	s := isolatedStore(t)
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	child := filepath.Join(repo, "child")
	require.NoError(t, os.Mkdir(child, 0o700))
	require.NoError(t, s.Save(Identity{Name: "work", Host: "github.com", Account: "alice"}, false))
	require.NoError(t, s.Bind(Binding{Identity: "work", Directory: child}, "/path/to/gh"))

	// When gh is invoked from that repository subdirectory
	selected, err := s.Resolve("github.com", "", child)

	// Then both gh and Git use a directory choice starting at the repository root
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, "work", selected.Name)
	assert.Equal(t, "alice\n", runGit(t, repo, "config", "credential.https://github.com.username"))
}
