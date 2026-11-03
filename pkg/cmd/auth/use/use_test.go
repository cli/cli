package use

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/identity"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUseDoesNotSaveScopeWhenGlobalIncludeCannotBeInstalled(t *testing.T) {
	// Given a saved identity and another writer holding Git's global config lock
	store, f := useFixture(t)
	require.NoError(t, store.Save(identity.Identity{Name: "work", Host: "github.com", Account: "alice"}, false))
	t.Chdir(t.TempDir())
	familyGit(t, ".", "init", "-q")
	require.NoError(t, os.WriteFile(os.Getenv("GIT_CONFIG_GLOBAL")+".lock", nil, 0o600))
	cmd := NewCmdUse(f, nil)
	cmd.SetOut(f.IOStreams.ErrOut)
	cmd.SetErr(f.IOStreams.ErrOut)
	cmd.SetArgs([]string{"work"})

	// When applying the identity without being able to configure independently invoked Git
	err := cmd.Execute()

	// Then setup fails and no gh-only scope is left behind
	require.ErrorContains(t, err, "could not lock config file")
	assert.NoFileExists(t, filepath.Join(os.Getenv("GH_CONFIG_DIR"), "auth.gitconfig"))
	choices, err := store.Bindings()
	require.NoError(t, err)
	assert.Empty(t, choices)
}

func useFixture(t *testing.T) (*identity.Store, *cmdutil.Factory) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("persistent identities are not supported on Windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GH_CONFIG_DIR", filepath.Join(home, "gh"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, ".gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, key := range []string{"GH_TOKEN", "GITHUB_TOKEN", "GIT_DIR", "GIT_WORK_TREE", "GIT_SSH", "GIT_SSH_COMMAND", "GIT_SSH_VARIANT", "GH_IDENTITY_SSH_COMMAND", "GH_IDENTITY_SSH_VARIANT", "GIT_CONFIG_COUNT", "GIT_CONFIG_PARAMETERS", "GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL", "GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(key, "")
		require.NoError(t, os.Unsetenv(key))
	}
	cfg := config.NewMockConfigFromString("hosts:\n  github.com:\n    users:\n      alice:\n      bob:\n")
	ios, _, _, _ := iostreams.Test()
	store, err := identity.Open()
	require.NoError(t, err)
	return store, &cmdutil.Factory{
		IOStreams: ios, ExecutablePath: "/path/to/gh",
		Config: func() (gh.Config, error) { return cfg, nil },
	}
}

func runUse(t *testing.T, f *cmdutil.Factory, args ...string) string {
	t.Helper()
	ios, _, stdout, stderr := iostreams.Test()
	factory := *f
	factory.IOStreams = ios
	cmd := NewCmdUse(&factory, nil)
	cmd.SetOut(stderr)
	cmd.SetErr(stderr)
	cmd.SetArgs(args)
	require.NoError(t, cmd.Execute(), "%s", stderr.String())
	return stdout.String()
}

func TestUseViewShowsSelectionWithoutChangingIt(t *testing.T) {
	// Given an identity selected for a directory
	store, f := useFixture(t)
	require.NoError(t, store.Save(identity.Identity{Name: "work", Host: "github.com", Account: "alice"}, false))
	t.Chdir(t.TempDir())
	familyGit(t, ".", "init", "-q")
	runUse(t, f, "work")

	// When explicitly viewing the selection
	output := runUse(t, f, "--view")

	// Then it is the same JSON as bare auth use, and later inspection is unchanged
	assert.JSONEq(t, `{"name":"work","host":"github.com","account":"alice"}`, output)
	assert.Equal(t, output, runUse(t, f))
}

func TestUseViewRejectsOtherActions(t *testing.T) {
	for _, args := range [][]string{
		{"--view", "work"}, {"--view", "--unset"}, {"--view", "--list"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			// Given an explicit view combined with another action
			ios, _, _, _ := iostreams.Test()
			cmd := NewCmdUse(&cmdutil.Factory{IOStreams: ios}, nil)
			cmd.SetArgs(args)
			cmd.SilenceUsage, cmd.SilenceErrors = true, true

			// When parsing that command
			err := cmd.Execute()

			// Then it fails before reading or changing configuration
			require.ErrorContains(t, err, "--view cannot be combined")
		})
	}
}

func TestUseOutsideRepositoryAppliesToFutureDescendants(t *testing.T) {
	// Given an identity and a directory that is not a repository
	store, f := useFixture(t)
	require.NoError(t, store.Save(identity.Identity{
		Name: "work", Host: "github.com", Account: "alice",
	}, false))
	work := t.TempDir()
	t.Chdir(work)

	// When auth use is invoked without a scope flag, before a child repository exists
	cmd := NewCmdUse(f, nil)
	cmd.SetArgs([]string{"work"})
	require.NoError(t, cmd.Execute())
	child := filepath.Join(work, "new-repository")
	require.NoError(t, os.Mkdir(child, 0o700))
	out, err := exec.Command("git", "-C", child, "init", "-q").CombinedOutput()
	require.NoError(t, err, "%s", out)

	// Then gh and independently invoked Git inherit the identity in the new repository
	selected, err := store.Resolve("github.com", "", child)
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, "work", selected.Name)
	out, err = exec.Command("git", "-C", child, "config", "gh-identity.github.com.name").CombinedOutput()
	require.NoError(t, err, "%s", out)
	assert.Equal(t, "work\n", string(out))
}

func TestUnsetRemovesDirectoryChoice(t *testing.T) {
	// Given an identity applied at a repository root
	store, f := useFixture(t)
	require.NoError(t, store.Save(identity.Identity{Name: "work", Host: "github.com", Account: "alice"}, false))
	repo := t.TempDir()
	out, err := exec.Command("git", "-C", repo, "init", "-q").CombinedOutput()
	require.NoError(t, err, "%s", out)
	t.Chdir(repo)
	runUse(t, f, "work")

	// When removing the choice through the current public command
	runUse(t, f, "--unset")

	// Then the directory choice no longer selects an identity
	selected, err := store.Resolve("github.com", "", repo)
	require.NoError(t, err)
	assert.Nil(t, selected)
}
