package logout

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/identity"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogoutWarnsAboutRetainedIdentitiesAndBindings(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("persistent identities are not supported on Windows")
	}
	// Given a signed-in account with a directory identity, using only isolated configuration and a fake keyring
	home := t.TempDir()
	t.Chdir(home)
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, ".gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, name := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_CONFIG_COUNT", "GIT_CONFIG_PARAMETERS", "GIT_SSH", "GIT_SSH_COMMAND", "GIT_SSH_VARIANT"} {
		t.Setenv(name, "")
		require.NoError(t, os.Unsetenv(name))
	}
	cfg, _ := config.NewIsolatedTestConfig(t, "hosts:\n  github.com:\n    user: alice\n    oauth_token: fixture-alice\n    users:\n      alice:\n        oauth_token: fixture-alice\n")
	s, err := identity.Open()
	require.NoError(t, err)
	require.NoError(t, s.Save(identity.Identity{Name: "work", Host: "github.com", Account: "alice", SSHHost: "work-ssh"}, false))
	require.NoError(t, s.Bind(identity.Binding{Identity: "work", Directory: home}, "/path/to/gh"))
	ios, _, stdout, stderr := iostreams.Test()
	cmd := NewCmdLogout(&cmdutil.Factory{IOStreams: ios, Config: func() (gh.Config, error) { return cfg, nil }}, nil)
	cmd.Flags().Bool("help", false, "")
	cmd.SetArgs([]string{"--hostname", "github.com", "--user", "alice"})

	// When logging out normally
	require.NoError(t, cmd.Execute())

	// Then credentials are removed but configuration remains, with actionable guidance and SSH caveats
	assert.Empty(t, stdout.String())
	assert.Empty(t, cfg.Authentication().UsersForHost("github.com"))
	assert.Contains(t, stderr.String(), `Identity "work" still references this account`)
	assert.Contains(t, stderr.String(), "gh auth identity delete work")
	assert.Contains(t, stderr.String(), "SSH keys and Git attribution are unchanged")
	assert.FileExists(t, s.IdentityPath("work"))
	bindings, err := s.Bindings()
	require.NoError(t, err)
	assert.Len(t, bindings, 1)
}
