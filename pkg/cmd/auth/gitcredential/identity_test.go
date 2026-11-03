package login

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	ghconfig "github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/identity"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCredentialHelperUsesBoundAccountRatherThanActiveAccount(t *testing.T) {
	isolateIdentityCredential(t)
	// Given an repository identity selecting alice while bob is globally active
	home := t.TempDir()
	require.NoError(t, exec.Command("git", "init", "-q", home).Run())
	t.Chdir(home)
	t.Setenv("HOME", home)
	t.Setenv("GH_CONFIG_DIR", filepath.Join(home, "gh"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, ".gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GIT_SSH_COMMAND", "")
	t.Setenv("GIT_SSH", "")
	s, err := identity.Open()
	require.NoError(t, err)
	require.NoError(t, s.Save(identity.Identity{Name: "work", Host: "github.com", Account: "alice"}, false))
	require.NoError(t, s.Bind(identity.Binding{Identity: "work", Directory: home}, "/path/to/gh"))
	ios, input, output, _ := iostreams.Test()
	fmt.Fprint(input, "protocol=https\nhost=github.com\npath=company/project.git\n\n")
	opts := &CredentialOptions{
		IO: ios, Operation: "get",
		Config: func() (config, error) {
			return ghconfig.NewMockConfigFromString(`
hosts:
  github.com:
    user: bob
    oauth_token: bob-token
    users:
      alice:
        oauth_token: alice-token
`).Authentication(), nil
		},
	}

	// When Git requests an HTTPS credential
	err = helperRun(opts)

	// Then the helper returns alice's credential, never bob's
	require.NoError(t, err)
	assert.Contains(t, output.String(), "username=alice\n")
	assert.Contains(t, output.String(), strings.Join([]string{"password=", "alice-token\n"}, ""))
	assert.NotContains(t, output.String(), "bob-token")
}
