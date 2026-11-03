package use

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cli/cli/v2/internal/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func familyGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)
	return strings.TrimSpace(string(out))
}

func TestUseFromAnyWorktreeSelectsEntireRepositoryFamily(t *testing.T) {
	// Given a main checkout, an external linked worktree, and an unrelated nested repository
	s, f := useFixture(t)
	main := t.TempDir()
	familyGit(t, main, "init", "-q")
	familyGit(t, main, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.test",
		"-c", "commit.gpgsign=false", "commit", "--allow-empty", "-qm", "initial")
	linked := filepath.Join(t.TempDir(), "linked")
	familyGit(t, main, "worktree", "add", "--detach", linked)
	nested := filepath.Join(main, "unrelated")
	require.NoError(t, os.Mkdir(nested, 0o700))
	familyGit(t, nested, "init", "-q")
	for _, id := range []identity.Identity{
		{Name: "work", Host: "github.com", Account: "alice", SSHHost: "work-ssh", GitUserName: "Alice", GitUserEmail: "work@example.test"},
		{Name: "personal", Host: "github.com", Account: "bob", SSHHost: "personal-ssh", GitUserName: "Bob", GitUserEmail: "personal@example.test"},
	} {
		require.NoError(t, s.Save(id, false))
	}
	t.Chdir(linked)

	// When choosing an identity from a linked worktree and creating another worktree later
	runUse(t, f, "work")
	future := filepath.Join(t.TempDir(), "future")
	familyGit(t, main, "worktree", "add", "--detach", future)

	// Then all family members share gh selection, Git attribution, and SSH routing, but not the nested repo
	for _, dir := range []string{main, linked, future} {
		var selected identity.Identity
		require.NoError(t, json.Unmarshal([]byte(runUse(t, f, "--view", "--directory", dir)), &selected))
		assert.Equal(t, "work", selected.Name)
		assert.Equal(t, "work@example.test", familyGit(t, dir, "config", "user.email"))
		args, err := s.SSHArguments([]string{"git@github.com", "git-upload-pack 'company/project.git'"}, dir)
		require.NoError(t, err)
		assert.Equal(t, "git@work-ssh", args[0])
	}
	assert.JSONEq(t, "null", runUse(t, f, "--view", "--directory", nested))
	var bindings []map[string]any
	require.NoError(t, json.Unmarshal([]byte(runUse(t, f, "--list")), &bindings))
	require.Len(t, bindings, 1)
	assert.Equal(t, familyGit(t, main, "rev-parse", "--path-format=absolute", "--git-common-dir"), bindings[0]["repository"])
	assert.NotContains(t, bindings[0], "directory")

	// When applying from another family member, the same choice changes everywhere
	runUse(t, f, "personal", "--directory", future)
	require.NoError(t, json.Unmarshal([]byte(runUse(t, f, "--list")), &bindings))
	require.Len(t, bindings, 1)
	assert.Equal(t, "personal@example.test", familyGit(t, main, "config", "user.email"))
	runUse(t, f, "--unset", "--directory", main)
	assert.JSONEq(t, "null", runUse(t, f, "--view", "--directory", linked))
}

func TestDirectoryChoicesFollowMainRepositoryNotWorktreeLocation(t *testing.T) {
	// Given a repository in a work tree and an external linked checkout located under a personal tree
	s, f := useFixture(t)
	work := t.TempDir()
	personal := t.TempDir()
	main := filepath.Join(work, "project")
	require.NoError(t, os.Mkdir(main, 0o700))
	familyGit(t, main, "init", "-q")
	familyGit(t, main, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.test", "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-qm", "initial")
	linked := filepath.Join(personal, "agent")
	familyGit(t, main, "worktree", "add", "--detach", linked)
	for _, id := range []identity.Identity{
		{Name: "work", Host: "github.com", Account: "alice", SSHHost: "work-ssh", GitUserName: "Alice", GitUserEmail: "work@example.test"},
		{Name: "personal", Host: "github.com", Account: "bob", SSHHost: "personal-ssh", GitUserName: "Bob", GitUserEmail: "personal@example.test"},
		{Name: "specific", Host: "github.com", Account: "bob", SSHHost: "specific-ssh", GitUserName: "Specific", GitUserEmail: "specific@example.test"},
	} {
		require.NoError(t, s.Save(id, false))
	}

	// When directory choices and an owner fallback are set outside any repository
	t.Chdir(work)
	runUse(t, f, "work")
	runUse(t, f, "personal", "--directory", personal)
	runUse(t, f, "personal", "--owner", "company")

	// Then the external worktree inherits the main repository's tree, not its checkout location or owner
	assert.Contains(t, runUse(t, f, "--view", "--directory", linked), `"name": "work"`)
	assert.Equal(t, "work@example.test", familyGit(t, linked, "config", "user.email"))
	args, err := s.SSHArguments([]string{"git@github.com", "git-upload-pack 'company/project.git'"}, linked)
	require.NoError(t, err)
	assert.Equal(t, "git@work-ssh", args[0])
	runUse(t, f, "specific", "--directory", linked)
	assert.Equal(t, "specific@example.test", familyGit(t, main, "config", "user.email"))
	t.Chdir(linked)
	runUse(t, f, "--unset")
	assert.Equal(t, "work@example.test", familyGit(t, main, "config", "user.email"))
	assert.Contains(t, runUse(t, f, "--view"), `"name": "work"`)
}

func TestFamilySetupDoesNotIgnoreSiblingSSHOverride(t *testing.T) {
	// Given a sibling worktree with an explicit SSH command that bypasses the global dispatcher
	s, f := useFixture(t)
	main := t.TempDir()
	familyGit(t, main, "init", "-q")
	familyGit(t, main, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.test", "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-qm", "initial")
	linked := filepath.Join(t.TempDir(), "linked")
	familyGit(t, main, "worktree", "add", "--detach", linked)
	familyGit(t, main, "config", "extensions.worktreeConfig", "true")
	familyGit(t, linked, "config", "--worktree", "core.sshCommand", "ssh -F sibling.conf")
	require.NoError(t, s.Save(identity.Identity{Name: "work", Host: "github.com", Account: "alice", SSHHost: "work-ssh"}, false))
	t.Chdir(main)
	cmd := NewCmdUse(f, nil)
	cmd.SilenceUsage, cmd.SilenceErrors = true, true
	cmd.SetArgs([]string{"work"})

	// When setup from the main checkout would claim to configure the entire family
	err := cmd.Execute()

	// Then it fails without changing sibling settings or installing a misleading family choice
	require.ErrorContains(t, err, "core.sshCommand")
	assert.Equal(t, "ssh -F sibling.conf", familyGit(t, linked, "config", "core.sshCommand"))
	bindings, err := s.Bindings()
	require.NoError(t, err)
	assert.Empty(t, bindings)
}
