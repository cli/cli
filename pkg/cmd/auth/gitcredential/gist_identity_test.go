package login

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	ghconfig "github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/identity"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGistCredentialHelperUsesCanonicalHostIdentity(t *testing.T) {
	isolateIdentityCredential(t)
	// Given an Alice directory identity while Bob is globally active
	home := t.TempDir()
	t.Chdir(home)
	t.Setenv("HOME", home)
	t.Setenv("GH_CONFIG_DIR", filepath.Join(home, "gh"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, ".gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, key := range []string{"GH_TOKEN", "GITHUB_TOKEN", "GH_ENTERPRISE_TOKEN", "GITHUB_ENTERPRISE_TOKEN", "GIT_SSH_COMMAND", "GIT_SSH", "GIT_SSH_VARIANT", "GH_IDENTITY_ACCOUNT", "GH_IDENTITY_HOST"} {
		t.Setenv(key, "")
	}
	s, err := identity.Open()
	require.NoError(t, err)
	require.NoError(t, s.Save(identity.Identity{Name: "work", Host: "github.com", Account: "alice"}, false))
	require.NoError(t, s.Bind(identity.Binding{Identity: "work", Directory: home}, "/path/to/gh"))
	ios, input, output, _ := iostreams.Test()
	fmt.Fprint(input, "protocol=https\nhost=gist.github.com\npath=bob/123.git\n\n")
	opts := &CredentialOptions{IO: ios, Operation: "get", Config: func() (config, error) {
		return ghconfig.NewMockConfigFromString(`
hosts:
  github.com:
    user: bob
    oauth_token: bob-token
    users:
      alice:
        oauth_token: alice-token
`).Authentication(), nil
	}}

	// When Git requests a gist credential through the same helper
	err = helperRun(opts)

	// Then the canonical host identity is selected, not the other active account
	require.NoError(t, err)
	assert.Contains(t, output.String(), "username=alice\n")
	assert.NotContains(t, output.String(), "bob-token")
}

func isolateIdentityCredential(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("persistent identities are not supported on Windows")
	}
	t.Chdir(t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	for _, name := range []string{"GIT_CONFIG", "GIT_CONFIG_COUNT", "GIT_CONFIG_PARAMETERS", "GIT_CONFIG_SYSTEM", "GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR"} {
		t.Setenv(name, "")
		require.NoError(t, os.Unsetenv(name))
	}
}
