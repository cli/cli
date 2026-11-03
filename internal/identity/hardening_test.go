package identity

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/kballard/go-shellquote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnsetDeletedDirectoryPreservesOtherChoices(t *testing.T) {
	// Given two choices using one identity, one of whose directories has been removed
	s := isolatedStore(t)
	require.NoError(t, s.Save(Identity{Name: "work", Host: "github.com", Account: "alice"}, false))
	parent := t.TempDir()
	removed := filepath.Join(parent, "removed")
	require.NoError(t, os.Mkdir(removed, 0o700))
	require.NoError(t, s.Bind(Binding{Identity: "work", Directory: removed}, "/path/to/gh"))
	require.NoError(t, s.Bind(Binding{Identity: "work", Owner: "company"}, "/path/to/gh"))
	require.NoError(t, os.Remove(removed))

	// When removing the stale choice by its original path
	err := s.Unbind(Binding{Host: "github.com", Directory: removed}, "/path/to/gh")

	// Then it can be removed without deleting the reusable identity or the other choice
	require.NoError(t, err)
	bindings, err := s.Bindings()
	require.NoError(t, err)
	require.Len(t, bindings, 1)
	assert.Equal(t, "company", bindings[0].Owner)
}

func TestPersistentIdentityRefusesBareRepositoryWithoutChangingConfig(t *testing.T) {
	// Given a bare repository where the working-tree identity contract cannot apply
	s := isolatedStore(t)
	dir := t.TempDir()
	runGit(t, dir, "init", "--bare", "-q")
	require.NoError(t, s.Save(Identity{Name: "work", Host: "github.com", Account: "alice"}, false))

	// When attempting a directory binding there
	err := s.Bind(Binding{Identity: "work", Directory: dir}, "/path/to/gh")

	// Then it fails before installing any global integration
	require.ErrorContains(t, err, "bare repositories")
	bindings, err := s.Bindings()
	require.NoError(t, err)
	assert.Empty(t, bindings)
}

func TestPersistentIdentityDoesNotReplaceCustomCredentialHelpers(t *testing.T) {
	// Given a user's existing HTTPS credential helper for the selected host
	s := isolatedStore(t)
	dir := t.TempDir()
	runGit(t, dir, "config", "--global", "credential.https://github.com.helper", "company-credential-helper")
	require.NoError(t, s.Save(Identity{Name: "work", Host: "github.com", Account: "alice"}, false))

	// When applying an identity would override the existing helper chain
	err := s.Bind(Binding{Identity: "work", Directory: dir}, "/path/to/gh")

	// Then the prototype refuses rather than taking ownership of that helper
	require.ErrorContains(t, err, "existing HTTPS credential helper")
	assert.Equal(t, "company-credential-helper\n", runGit(t, dir, "config", "--global", "credential.https://github.com.helper"))
}

func TestCredentialHelperResetAllowsExistingGhSetup(t *testing.T) {
	// Given a system keychain helper already reset by the user's gh setup
	s := isolatedStore(t)
	dir := t.TempDir()
	system := filepath.Join(dir, "system.gitconfig")
	require.NoError(t, os.WriteFile(system, []byte("[credential]\nhelper = osxkeychain\n"), 0o600))
	t.Setenv("GIT_CONFIG_SYSTEM", system)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "0")
	runGit(t, dir, "config", "--global", "--add", "credential.https://github.com.helper", "")
	runGit(t, dir, "config", "--global", "--add", "credential.https://github.com.helper", "!gh auth git-credential")
	require.NoError(t, s.Save(Identity{Name: "work", Host: "github.com", Account: "alice"}, false))

	// When applying an identity using the already-gh-managed credential setup
	err := s.Bind(Binding{Identity: "work", Directory: dir}, "/path/to/gh")

	// Then the inactive system helper does not make a valid setup unusable
	require.NoError(t, err)
}

func TestPersistentIdentityRefusesSeparateGitDirectory(t *testing.T) {
	// Given a checkout whose Git metadata is outside the directory tree
	s := isolatedStore(t)
	dir := t.TempDir()
	gitDir := filepath.Join(t.TempDir(), "metadata")
	runGit(t, dir, "init", "--separate-git-dir", gitDir, "-q")
	require.NoError(t, s.Save(Identity{Name: "work", Host: "github.com", Account: "alice"}, false))

	// When binding the checkout would not activate its native Git include
	err := s.Bind(Binding{Identity: "work", Directory: dir}, "/path/to/gh")

	// Then the unsupported layout is rejected instead of giving gh and Git different settings
	require.ErrorContains(t, err, "separate Git directories")
}

func TestPersistentIdentityRefusesRepositoryUsingSubmodulesAtSetup(t *testing.T) {
	// Given a standalone repository whose Git operations may recurse into submodules
	s := isolatedStore(t)
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitmodules"), []byte("[submodule \"library\"]\npath = vendor/library\nurl = https://github.com/example/library.git\n"), 0o600))
	require.NoError(t, s.Save(Identity{Name: "work", Host: "github.com", Account: "alice"}, false))

	// When applying an identity to that repository
	err := s.Bind(Binding{Identity: "work", Directory: dir}, "/path/to/gh")

	// Then incompatibility is reported at setup instead of during a later submodule fetch
	require.ErrorContains(t, err, "repositories that use submodules")
	bindings, err := s.Bindings()
	require.NoError(t, err)
	assert.Empty(t, bindings)
}

func TestOldGitCannotInstallUnsupportedConditionalConfiguration(t *testing.T) {
	// Given a Git version without hasconfig include conditions
	s := isolatedStore(t)
	require.NoError(t, s.Save(Identity{Name: "work", Host: "github.com", Account: "alice"}, false))
	realGit, err := exec.LookPath("git")
	require.NoError(t, err)
	tools := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tools, "git"), []byte("#!/bin/sh\nif [ \"$1\" = version ]; then printf 'git version 2.35.1\\n'; exit 0; fi\nexec "+shellquote.Join(realGit)+" \"$@\"\n"), 0o700))
	t.Setenv("PATH", tools+string(os.PathListSeparator)+os.Getenv("PATH"))

	// When setup would otherwise install an owner choice
	err = s.Bind(Binding{Identity: "work", Owner: "company"}, "/path/to/gh")

	// Then it fails explicitly before installing configuration Git would silently ignore
	require.ErrorContains(t, err, "Git 2.36 or newer")
	assert.NoFileExists(t, filepath.Join(filepath.Dir(s.Dir), "auth.gitconfig"))
}
