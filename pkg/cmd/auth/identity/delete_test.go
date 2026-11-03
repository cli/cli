package identity

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	identities "github.com/cli/cli/v2/internal/identity"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResetExperimentUsesTypedAllIdentitiesConfirmation(t *testing.T) {
	// Given an existing experiment and an interactive user who cancels
	f := identityFactory(t)
	t.Chdir(t.TempDir())
	runIdentity(t, f, "create", "work", "--account", "alice")
	f.IOStreams.SetStdinTTY(true)
	f.IOStreams.SetStdoutTTY(true)
	f.Prompter = &prompter.PrompterMock{ConfirmDeletionFunc: func(value string) error {
		assert.Equal(t, "all identities", value)
		return cmdutil.CancelError
	}}
	cmd := NewCmdResetExperiment(f)
	cmd.SilenceErrors, cmd.SilenceUsage = true, true

	// When the dedicated reset command asks for the same confirmation as bulk deletion
	err := cmd.Execute()

	// Then cancellation preserves the identity
	require.ErrorIs(t, err, cmdutil.CancelError)
	assert.Contains(t, runIdentity(t, f, "view", "work"), `"name": "work"`)
}

func TestDeleteIdentityRequiresExplicitConfirmation(t *testing.T) {
	// Given a saved identity and no terminal
	f := identityFactory(t)
	t.Chdir(t.TempDir())
	runIdentity(t, f, "create", "work", "--account", "alice")
	cmd := NewCmdIdentity(f)
	cmd.SilenceErrors, cmd.SilenceUsage = true, true
	cmd.SetArgs([]string{"delete", "work"})

	// When deletion has not been confirmed
	err := cmd.Execute()

	// Then it fails without removing the identity
	require.ErrorContains(t, err, "--confirm required when not running interactively")
	assert.Contains(t, runIdentity(t, f, "view", "work"), `"name": "work"`)
}

func TestDeleteIdentityWarningDistinguishesBindingsFromRepositories(t *testing.T) {
	// Given an identity used by owner, directory, and repository bindings
	f := identityFactory(t)
	t.Chdir(t.TempDir())
	runIdentity(t, f, "create", "work", "--account", "alice")
	repo := filepath.Join(t.TempDir(), "checkout")
	out, err := exec.Command("git", "init", "-q", repo).CombinedOutput()
	require.NoError(t, err, "%s", out)
	repo, err = filepath.EvalSymlinks(repo)
	require.NoError(t, err)
	directory, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	s, err := identities.Open()
	require.NoError(t, err)
	require.NoError(t, s.Bind(identities.Binding{Identity: "work", Owner: "company"}, f.ExecutablePath))
	require.NoError(t, s.Bind(identities.Binding{Identity: "work", Directory: directory}, f.ExecutablePath))
	require.NoError(t, s.Bind(identities.Binding{Identity: "work", Directory: repo}, f.ExecutablePath))
	ios, _, stdout, stderr := iostreams.Test()
	ios.SetStdinTTY(true)
	ios.SetStdoutTTY(true)
	f.IOStreams = ios
	f.Prompter = &prompter.PrompterMock{ConfirmDeletionFunc: func(value string) error {
		assert.Equal(t, "work", value)
		assert.Contains(t, stderr.String(), "  Remove binding for repository "+repo+" (all worktrees) on github.com\n")
		assert.Contains(t, stderr.String(), "  Remove binding for directory "+directory+" on github.com\n")
		assert.Contains(t, stderr.String(), "  Remove binding for owner company on github.com\n")
		assert.Contains(t, stderr.String(), "Repository files, commits, and worktrees will not be deleted.\n")
		assert.NotContains(t, stderr.String(), filepath.Join(repo, ".git"))
		return cmdutil.CancelError
	}}
	cmd := NewCmdIdentity(f)
	cmd.SilenceErrors, cmd.SilenceUsage = true, true
	cmd.SetArgs([]string{"delete", "work"})

	// When reviewing deletion before deciding whether to confirm
	err = cmd.Execute()

	// Then the warning describes binding removal, not deletion of repositories or their metadata
	require.ErrorIs(t, err, cmdutil.CancelError)
	assert.Empty(t, stdout.String())
}

func TestDeleteIdentityCascadesOnlyItsOwnBindings(t *testing.T) {
	// Given two identities bound to different owners
	f := identityFactory(t)
	t.Chdir(t.TempDir())
	s, err := identities.Open()
	require.NoError(t, err)
	for _, name := range []string{"work", "personal"} {
		runIdentity(t, f, "create", name, "--account", "alice")
		require.NoError(t, s.Bind(identities.Binding{Identity: name, Owner: name}, f.ExecutablePath))
	}
	ios, _, stdout, stderr := iostreams.Test()
	ios.SetStdinTTY(true)
	ios.SetStdoutTTY(true)
	f.IOStreams = ios
	f.Prompter = &prompter.PrompterMock{ConfirmDeletionFunc: func(value string) error {
		assert.Equal(t, "work", value)
		assert.Contains(t, stderr.String(), "owner work")
		assert.Contains(t, stderr.String(), "Saved accounts")
		return nil
	}}
	cmd := NewCmdIdentity(f)
	cmd.SetArgs([]string{"delete", "work"})

	// When the user retypes the identity name to confirm deletion
	require.NoError(t, cmd.Execute())

	// Then only that identity and its bindings are removed
	assert.Empty(t, stdout.String())
	assert.NoFileExists(t, s.IdentityPath("work"))
	assert.FileExists(t, s.IdentityPath("personal"))
	bindings, err := s.Bindings()
	require.NoError(t, err)
	require.Len(t, bindings, 1)
	assert.Equal(t, "personal", bindings[0].Identity)
}

func TestDeleteAllCleansManagedIntegrationAndPreservesUserSettings(t *testing.T) {
	// Given managed identities alongside unrelated configuration and files
	f := identityFactory(t)
	t.Chdir(t.TempDir())
	s, err := identities.Open()
	require.NoError(t, err)
	global := os.Getenv("GIT_CONFIG_GLOBAL")
	require.NoError(t, os.WriteFile(global, []byte("[user]\nname = Original\n"), 0o600))
	runIdentity(t, f, "create", "work", "--account", "alice")
	require.NoError(t, s.Bind(identities.Binding{Identity: "work", Owner: "company"}, f.ExecutablePath))
	unrelated := filepath.Join(s.Dir, "notes.txt")
	require.NoError(t, os.WriteFile(unrelated, []byte("keep me"), 0o600))
	beforeUser, err := f.Config()
	require.NoError(t, err)
	users := beforeUser.Authentication().UsersForHost("github.com")

	// When all identity configuration is explicitly deleted
	runIdentity(t, f, "delete", "--all", "--confirm")

	// Then no managed integration remains, and the user's original settings/accounts survive
	assert.JSONEq(t, "[]", runIdentity(t, f, "list"))
	assert.NoFileExists(t, filepath.Join(filepath.Dir(s.Dir), "auth.gitconfig"))
	assert.FileExists(t, unrelated)
	out, err := exec.Command("git", "config", "--global", "--get", "user.name").Output()
	require.NoError(t, err)
	assert.Equal(t, "Original\n", string(out))
	out, err = exec.Command("git", "config", "--global", "--get-all", "include.path").Output()
	require.Error(t, err)
	assert.Empty(t, string(out))
	assert.Equal(t, users, beforeUser.Authentication().UsersForHost("github.com"))
	runIdentity(t, f, "delete", "--all", "--confirm")
}

func TestCancelledIdentityDeletionKeepsBindings(t *testing.T) {
	// Given a bound identity and a cancelled typed confirmation
	f := identityFactory(t)
	t.Chdir(t.TempDir())
	s, err := identities.Open()
	require.NoError(t, err)
	runIdentity(t, f, "create", "work", "--account", "alice")
	require.NoError(t, s.Bind(identities.Binding{Identity: "work", Owner: "company"}, f.ExecutablePath))
	f.IOStreams.SetStdinTTY(true)
	f.IOStreams.SetStdoutTTY(true)
	f.Prompter = &prompter.PrompterMock{ConfirmDeletionFunc: func(value string) error {
		assert.Equal(t, "all identities", value)
		return cmdutil.CancelError
	}}
	cmd := NewCmdIdentity(f)
	cmd.SilenceErrors, cmd.SilenceUsage = true, true
	cmd.SetArgs([]string{"delete", "--all"})

	// When the user does not confirm
	err = cmd.Execute()

	// Then no identity or scope is removed
	require.ErrorIs(t, err, cmdutil.CancelError)
	assert.FileExists(t, s.IdentityPath("work"))
	bindings, err := s.Bindings()
	require.NoError(t, err)
	assert.Len(t, bindings, 1)
}

func TestDeleteAllRollsBackWhenGitConfigurationIsLocked(t *testing.T) {
	// Given a bound identity and a Git config locked by another writer
	f := identityFactory(t)
	t.Chdir(t.TempDir())
	s, err := identities.Open()
	require.NoError(t, err)
	runIdentity(t, f, "create", "work", "--account", "alice")
	require.NoError(t, s.Bind(identities.Binding{Identity: "work", Owner: "company"}, f.ExecutablePath))
	require.NoError(t, os.WriteFile(os.Getenv("GIT_CONFIG_GLOBAL")+".lock", nil, 0o600))
	cmd := NewCmdIdentity(f)
	cmd.SilenceErrors, cmd.SilenceUsage = true, true
	cmd.SetArgs([]string{"delete", "--all", "--confirm"})

	// When cleanup cannot detach the managed Git include
	err = cmd.Execute()

	// Then the identity and its configuration are restored, not left pointing at a missing file
	require.ErrorContains(t, err, "could not lock config file")
	assert.FileExists(t, s.IdentityPath("work"))
	assert.FileExists(t, filepath.Join(filepath.Dir(s.Dir), "auth.gitconfig"))
	bindings, err := s.Bindings()
	require.NoError(t, err)
	require.Len(t, bindings, 1)
	assert.Equal(t, "work", bindings[0].Identity)
}

func TestDeleteAllPreservesUnexpectedSettingsInManagedFile(t *testing.T) {
	// Given someone has added unrelated settings to the generated file
	f := identityFactory(t)
	t.Chdir(t.TempDir())
	s, err := identities.Open()
	require.NoError(t, err)
	runIdentity(t, f, "create", "work", "--account", "alice")
	require.NoError(t, s.Bind(identities.Binding{Identity: "work", Owner: "company"}, f.ExecutablePath))
	path := filepath.Join(filepath.Dir(s.Dir), "auth.gitconfig")
	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	contents = append(contents, []byte("\n[user]\nname = Do not delete\n")...)
	require.NoError(t, os.WriteFile(path, contents, 0o600))
	cmd := NewCmdIdentity(f)
	cmd.SilenceUsage, cmd.SilenceErrors = true, true
	cmd.SetArgs([]string{"delete", "--all", "--confirm"})

	// When bulk cleanup encounters settings it does not own
	err = cmd.Execute()

	// Then it explains the conflict and keeps the configuration and identity intact
	require.ErrorContains(t, err, "settings not managed by gh")
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, contents, after)
	assert.FileExists(t, s.IdentityPath("work"))
}
