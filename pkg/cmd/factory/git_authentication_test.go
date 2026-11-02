package factory

import (
	"context"
	"testing"

	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const gitAccountConfig = `
hosts:
  github.com:
    user: alice
    oauth_token: alice-token
    users:
      alice:
        oauth_token: alice-token
      bob:
        oauth_token: bob-token
  enterprise.example:
    user: carol
    oauth_token: carol-token
    users:
      carol:
        oauth_token: carol-token
`

func TestGitClientForwardsSelectionWithoutUsernameArguments(t *testing.T) {
	// Given a Git client copied before the command selects an account
	cfg, _ := config.NewIsolatedTestConfig(t, gitAccountConfig)
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{Config: func() (gh.Config, error) { return cfg, nil }, IOStreams: ios, ExecutablePath: "/tools/gh"}
	client := newGitClient(f).Copy()
	client.GitPath = "/tools/git"
	cfg.Authentication().SetAccountOverride("github.com", "bob")

	// When constructing a command that may access multiple remotes
	cmd, err := client.AuthenticatedCommand(context.Background(), git.AllMatchingCredentialsPattern, "fetch", "--all")

	// Then Git receives one helper and the child context, without preselecting a username for any host
	require.NoError(t, err)
	assert.Equal(t, []string{"/tools/git", "-c", "credential.helper=", "-c",
		`credential.helper=!"/tools/gh" auth git-credential`, "fetch", "--all"}, cmd.Args)
	assert.Contains(t, cmd.Env, "GH_IDENTITY_ACCOUNT=bob")
	assert.NotContains(t, cmd.Env, "bob-token")
}

func TestGitClientLeavesOrdinarySSHUnchanged(t *testing.T) {
	// Given ordinary saved authentication and an existing SSH command
	cfg, _ := config.NewIsolatedTestConfig(t, gitAccountConfig)
	t.Setenv("GIT_SSH_COMMAND", "ssh -F company.conf")
	t.Setenv("GIT_SSH_VARIANT", "ssh")
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{Config: func() (gh.Config, error) { return cfg, nil }, IOStreams: ios, ExecutablePath: "/tools/gh"}
	client := newGitClient(f)
	client.GitPath = "/tools/git"

	// When constructing an authenticated command without an explicit selection
	cmd, err := client.AuthenticatedCommand(context.Background(), git.AllMatchingCredentialsPattern, "fetch")

	// Then SSH continues using the user's own command rather than gh's account-aware helper
	require.NoError(t, err)
	assert.Contains(t, cmd.Env, "GIT_SSH_COMMAND=ssh -F company.conf")
	assert.Contains(t, cmd.Env, "GIT_SSH_VARIANT=ssh")
}
