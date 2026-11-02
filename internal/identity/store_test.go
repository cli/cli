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

func TestRepositoryUsesLiveIdentity(t *testing.T) {
	// Given an identity referencing an account, SSH route, and commit metadata
	s := isolatedStore(t)
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	id := Identity{Name: "work", Host: "github.com", Account: "alice", SSHHost: "github-work", GitUserName: "Alice", GitUserEmail: "alice@work.test"}
	require.NoError(t, s.Save(id, false))

	// When that identity is applied to a repository and subsequently edited
	require.NoError(t, s.Bind(Binding{Identity: "work", Directory: repo}, "/path/to/gh"))
	id.GitUserEmail = "new@work.test"
	require.NoError(t, s.Save(id, true))
	got, err := s.Resolve("github.com", "", repo)

	// Then both gh selection and Git see the edited shared identity
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "alice", got.Account)
	assert.Equal(t, "new@work.test\n", runGit(t, repo, "config", "user.email"))
	assert.Equal(t, "work\n", runGit(t, repo, "config", "gh-identity.github.com.name"))
	contents, err := os.ReadFile(s.IdentityPath("work"))
	require.NoError(t, err)
	assert.NotContains(t, string(contents), "oauth_token")
}

func TestDeeperDirectoryOverridesTreeAndUnbindingRevealsIt(t *testing.T) {
	// Given overlapping owner, broad directory, and deeper directory choices
	s := isolatedStore(t)
	parent := t.TempDir()
	repo := filepath.Join(parent, "repo")
	require.NoError(t, os.Mkdir(repo, 0o700))
	runGit(t, repo, "init", "-q")
	runGit(t, repo, "remote", "add", "origin", "git@github.com:company/project.git")
	for _, id := range []Identity{
		{Name: "owner", Host: "github.com", Account: "owner", SSHHost: "github-owner", GitUserName: "Owner", GitUserEmail: "owner@test.invalid"},
		{Name: "tree", Host: "github.com", Account: "tree", SSHHost: "github-tree", GitUserName: "Tree", GitUserEmail: "tree@test.invalid"},
		{Name: "repo", Host: "github.com", Account: "repo", SSHHost: "github-repo", GitUserName: "Repo", GitUserEmail: "repo@test.invalid"},
	} {
		require.NoError(t, s.Save(id, false))
	}
	require.NoError(t, s.Bind(Binding{Identity: "owner", Owner: "company"}, "/path/to/gh"))
	require.NoError(t, s.Bind(Binding{Identity: "tree", Directory: parent}, "/path/to/gh"))
	require.NoError(t, s.Bind(Binding{Identity: "repo", Directory: repo}, "/path/to/gh"))
	selected, err := s.Resolve("github.com", "company", repo)
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, "repo", selected.Name)
	assert.Equal(t, "repo@test.invalid\n", runGit(t, repo, "config", "user.email"))

	// When removing only the deeper directory choice
	require.NoError(t, s.Unbind(Binding{Host: "github.com", Directory: repo}, "/path/to/gh"))
	selected, err = s.Resolve("github.com", "company", repo)

	// Then both Git and gh reveal the tree identity, ahead of the owner identity
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, "tree", selected.Name)
	assert.Equal(t, "tree@test.invalid\n", runGit(t, repo, "config", "user.email"))
}

func TestSwitchingToAccountOnlyRemovesPreviousCommitMetadata(t *testing.T) {
	// Given ordinary Git defaults and an explicitly applied commit identity
	s := isolatedStore(t)
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	runGit(t, repo, "config", "--global", "user.email", "ordinary@test.invalid")
	require.NoError(t, s.Save(Identity{Name: "work", Host: "github.com", Account: "alice", GitUserName: "Work", GitUserEmail: "work@test.invalid"}, false))
	require.NoError(t, s.Save(Identity{Name: "account-only", Host: "github.com", Account: "alice"}, false))
	require.NoError(t, s.Bind(Binding{Identity: "work", Directory: repo}, "/path/to/gh"))

	// When replacing that binding with an account-only identity
	require.NoError(t, s.Bind(Binding{Identity: "account-only", Directory: repo}, "/path/to/gh"))

	// Then the previous identity no longer contributes any commit metadata
	assert.Equal(t, "ordinary@test.invalid\n", runGit(t, repo, "config", "user.email"))
}

func TestExplicitGitDirectoryUsesSelectedRepository(t *testing.T) {
	// Given an explicit Git directory with a tree binding different from the owner's default
	s := isolatedStore(t)
	parent := t.TempDir()
	destination := filepath.Join(parent, "destination")
	require.NoError(t, os.Mkdir(destination, 0o700))
	runGit(t, destination, "init", "-q")
	require.NoError(t, s.Save(Identity{Name: "owner", Host: "github.com", Account: "owner"}, false))
	require.NoError(t, s.Save(Identity{Name: "tree", Host: "github.com", Account: "tree"}, false))
	require.NoError(t, s.Bind(Binding{Identity: "owner", Owner: "company"}, "/path/to/gh"))
	require.NoError(t, s.Bind(Binding{Identity: "tree", Directory: parent}, "/path/to/gh"))
	t.Setenv("GIT_DIR", filepath.Join(destination, ".git"))

	// When Git invokes its transport from elsewhere with the destination's GIT_DIR
	got, err := s.Resolve("github.com", "company", t.TempDir())

	// Then transport authentication follows the destination's tree
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "tree", got.Name)
}

func TestOwnerBindingMatchesCanonicalCapitalization(t *testing.T) {
	// Given an owner binding whose canonical remote uses mixed-case spelling
	s := isolatedStore(t)
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	runGit(t, repo, "remote", "add", "origin", "git@github.com:MyCompany/project.git")
	require.NoError(t, s.Save(Identity{Name: "work", Host: "github.com", Account: "alice", GitUserName: "Alice", GitUserEmail: "alice@test.invalid"}, false))
	require.NoError(t, s.Bind(Binding{Identity: "work", Owner: "mycompany"}, "/path/to/gh"))

	// When ordinary Git reads the conditional identity include
	email := runGit(t, repo, "config", "user.email")

	// Then Git uses the same case-insensitive owner matching as gh
	assert.Equal(t, "alice@test.invalid\n", email)
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
	require.NoError(t, s.Save(Identity{Name: "work", Host: "github.com", Account: "alice", GitUserName: "Alice", GitUserEmail: "alice@test.invalid"}, false))
	require.NoError(t, s.Bind(Binding{Identity: "work", Directory: child}, "/path/to/gh"))

	// When gh is invoked from that repository subdirectory
	selected, err := s.Resolve("github.com", "", child)

	// Then both gh and Git use a directory choice starting at the repository root
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, "work", selected.Name)
	assert.Equal(t, "alice@test.invalid\n", runGit(t, repo, "config", "user.email"))
}

func TestNestedDirectoryBindingOverridesAndRevealsParent(t *testing.T) {
	// Given two directory choices above a repository
	s := isolatedStore(t)
	parent := t.TempDir()
	nested := filepath.Join(parent, "team")
	repo := filepath.Join(nested, "project")
	require.NoError(t, os.MkdirAll(repo, 0o700))
	runGit(t, repo, "init", "-q")
	require.NoError(t, s.Save(Identity{Name: "parent", Host: "github.com", Account: "alice"}, false))
	require.NoError(t, s.Save(Identity{Name: "nested", Host: "github.com", Account: "bob"}, false))
	require.NoError(t, s.Bind(Binding{Identity: "parent", Directory: parent}, "/path/to/gh"))
	require.NoError(t, s.Bind(Binding{Identity: "nested", Directory: nested}, "/path/to/gh"))

	// When resolving the repository before and after removing only the deeper directory choice
	selected, err := s.Resolve("github.com", "", repo)
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, "nested", selected.Name)
	assert.Equal(t, "nested\n", runGit(t, repo, "config", "gh-identity.github.com.name"))
	require.NoError(t, s.Unbind(Binding{Host: "github.com", Directory: nested}, "/path/to/gh"))
	selected, err = s.Resolve("github.com", "", repo)

	// Then both gh and independent Git reveal the parent directory's identity
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, "parent", selected.Name)
	assert.Equal(t, "parent\n", runGit(t, repo, "config", "gh-identity.github.com.name"))
}

func TestConflictingOwnerFallbackRequiresRepositoryChoice(t *testing.T) {
	// Given a repository whose remotes select different owner identities
	s := isolatedStore(t)
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	for _, owner := range []string{"alice", "company"} {
		require.NoError(t, s.Save(Identity{Name: owner, Host: "github.com", Account: owner}, false))
		require.NoError(t, s.Bind(Binding{Identity: owner, Owner: owner}, "/path/to/gh"))
		runGit(t, repo, "remote", "add", owner, "https://github.com/"+owner+"/project.git")
	}

	// When no request owner or explicit repository choice disambiguates the remotes
	_, err := s.Resolve("github.com", "", repo)
	require.ErrorContains(t, err, "multiple owners select different identities")
	require.NoError(t, s.Bind(Binding{Identity: "alice", Directory: repo}, "/path/to/gh"))
	selected, err := s.Resolve("github.com", "", repo)

	// Then an explicit repository choice overrides both fallback owners
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, "alice", selected.Name)
}
