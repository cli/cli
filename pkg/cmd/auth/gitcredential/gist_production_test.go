package login

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"testing"

	ghconfig "github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/identity"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func productionGistFixture(t *testing.T, extraHost string) (gh.Config, *cmdutil.Factory, *bytes.Buffer) {
	t.Helper()
	isolateIdentityCredential(t)
	cfg, _ := ghconfig.NewIsolatedTestConfig(t, `
hosts:
  github.com:
    user: bob
    oauth_token: bob-token
    users:
      alice:
        oauth_token: alice-token
      bob:
        oauth_token: bob-token
`+extraHost)
	home := t.TempDir()
	require.NoError(t, exec.Command("git", "init", "-q", home).Run())
	t.Chdir(home)
	t.Chdir(home)
	t.Setenv("HOME", home)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, ".gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, key := range []string{"GIT_SSH", "GIT_SSH_COMMAND", "GIT_SSH_VARIANT", "GH_IDENTITY_ACCOUNT", "GH_IDENTITY_HOST"} {
		t.Setenv(key, "")
	}
	s, err := identity.Open()
	require.NoError(t, err)
	require.NoError(t, s.Save(identity.Identity{Name: "work", Host: "github.com", Account: "alice"}, false))
	require.NoError(t, s.Bind(identity.Binding{Identity: "work", Directory: home}, "/path/to/gh"))
	ios, input, output, _ := iostreams.Test()
	fmt.Fprint(input, "protocol=https\nhost=gist.github.com\npath=alice/123.git\n\n")
	return cfg, &cmdutil.Factory{IOStreams: ios, Config: func() (gh.Config, error) { return cfg, nil }}, output
}

func TestGistWithProductionConfigUsesPersistentIdentity(t *testing.T) {
	// Given real config normalization, Bob's active host token, and Alice's repository identity
	_, f, output := productionGistFixture(t, "")
	cmd := NewCmdCredential(f, nil)
	cmd.SetArgs([]string{"get"})

	// When Git requests a gist credential
	require.NoError(t, cmd.Execute())

	// Then normalization cannot mistake Bob's parent-host token for an independent gist credential
	assert.Contains(t, output.String(), "username=alice\n")
	assert.Contains(t, output.String(), "alice-token")
	assert.NotContains(t, output.String(), "bob-token")
}

func TestGistWithProductionConfigHonorsExplicitAccount(t *testing.T) {
	// Given Alice's repository identity and an explicit Bob override on the parent host
	_, f, output := productionGistFixture(t, "")
	t.Setenv("GH_IDENTITY_HOST", "github.com")
	t.Setenv("GH_IDENTITY_ACCOUNT", "bob")
	cmd := NewCmdCredential(f, nil)
	cmd.SetArgs([]string{"get"})

	// When Git requests a gist credential
	require.NoError(t, cmd.Execute())

	// Then the explicit account override reaches gist requests too
	assert.Contains(t, output.String(), "username=bob\n")
	assert.Contains(t, output.String(), "bob-token")
}

func TestGistWithIndependentRegistrationUsesExactHostAccount(t *testing.T) {
	// Given a genuinely separate gist registration in addition to parent-host identities
	_, f, output := productionGistFixture(t, `
  gist.github.com:
    user: carol
    oauth_token: gist-token
    users:
      carol:
        oauth_token: gist-token
`)
	cmd := NewCmdCredential(f, nil)
	cmd.SetArgs([]string{"get"})

	// When Git requests that host's credential
	require.NoError(t, cmd.Execute())

	// Then it uses the exact host's saved account, not the normalizing parent-host lookup
	assert.Contains(t, output.String(), "username=carol\n")
	assert.Contains(t, output.String(), "gist-token")
	assert.NotContains(t, output.String(), "bob-token")
}

func TestGistMissingBoundAccountCannotFallBackToActiveAccount(t *testing.T) {
	// Given Alice's retained identity after her credential is removed from the isolated config/keyring
	cfg, f, output := productionGistFixture(t, "")
	require.NoError(t, cfg.Authentication().Logout("github.com", "alice"))
	cmd := NewCmdCredential(f, nil)
	cmd.SilenceErrors, cmd.SilenceUsage = true, true
	cmd.SetArgs([]string{"get"})

	// When Git requests a gist credential
	err := cmd.Execute()

	// Then it fails closed rather than authenticating as Bob
	require.ErrorContains(t, err, "is not signed in")
	assert.Contains(t, output.String(), "quit=1")
	assert.NotContains(t, output.String(), "bob-token")
}
