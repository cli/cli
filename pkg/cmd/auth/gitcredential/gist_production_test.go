package login

import (
	"bytes"
	"fmt"
	"path/filepath"
	"testing"

	ghconfig "github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
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
	t.Chdir(home)
	t.Setenv("HOME", home)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, ".gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, key := range []string{"GIT_SSH", "GIT_SSH_COMMAND", "GIT_SSH_VARIANT", "GH_IDENTITY_ACCOUNT", "GH_IDENTITY_HOST"} {
		t.Setenv(key, "")
	}
	ios, input, output, _ := iostreams.Test()
	fmt.Fprint(input, "protocol=https\nhost=gist.github.com\npath=alice/123.git\n\n")
	return cfg, &cmdutil.Factory{IOStreams: ios, Config: func() (gh.Config, error) { return cfg, nil }}, output
}

func TestGistWithProductionConfigHonorsExplicitAccount(t *testing.T) {
	// Given saved accounts and an explicit Bob override on the parent host
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
	// Given a genuinely separate gist registration in addition to parent-host accounts
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
