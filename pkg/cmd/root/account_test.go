package root

import (
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestCmd creates a minimal cobra command with the "account" string flag,
// simulating the flag registration done in NewCmdRoot.
func newTestCmd(name string) *cobra.Command {
	cmd := &cobra.Command{Use: name}
	cmd.Flags().String("account", "", "")
	return cmd
}

// setupTestConfig creates an isolated test config with a logged-in user and
// returns the gh.Config interface value for use in tests.
func setupTestConfig(t *testing.T, hostname, username, token string) gh.Config {
	t.Helper()
	cfg, _ := config.NewIsolatedTestConfig(t)
	authCfg := cfg.Authentication()
	realAuthCfg, ok := authCfg.(*config.AuthConfig)
	require.True(t, ok, "expected *config.AuthConfig from Authentication()")
	_, err := realAuthCfg.Login(hostname, username, token, "", false)
	require.NoError(t, err)
	return cfg
}

func TestApplyAccountOverride_ValidAccountSetsOverride(t *testing.T) {
	// Given a config with a logged-in user
	cfg := setupTestConfig(t, "github.com", "monalisa", "test-token")

	cmd := newTestCmd("status")
	require.NoError(t, cmd.Flags().Set("account", "monalisa@github.com"))

	// When we apply the account override
	err := applyAccountOverride(cmd, cfg)

	// Then it should succeed with no error
	require.NoError(t, err)

	// Then the override should change which token is returned
	token, _ := cfg.Authentication().ActiveToken("github.com")
	require.Equal(t, "test-token", token)
}

func TestApplyAccountOverride_GHAccountEnvVar(t *testing.T) {
	// Given a config with a logged-in user
	cfg := setupTestConfig(t, "github.com", "monalisa", "test-token")

	// When GH_ACCOUNT env var is set (no flag)
	t.Setenv("GH_ACCOUNT", "monalisa@github.com")
	cmd := newTestCmd("status")

	err := applyAccountOverride(cmd, cfg)

	// Then it should succeed with no error (env var was picked up)
	require.NoError(t, err)
}

func TestApplyAccountOverride_FlagTakesPrecedenceOverEnvVar(t *testing.T) {
	// Given a config with two logged-in users on different hosts
	cfg := setupTestConfig(t, "github.com", "monalisa", "test-token")
	// Also log in as a second user
	authCfg, ok := cfg.Authentication().(*config.AuthConfig)
	require.True(t, ok)
	_, err := authCfg.Login("github.com", "hubot", "other-token", "", false)
	require.NoError(t, err)

	// When both the flag and env var are set
	t.Setenv("GH_ACCOUNT", "hubot@github.com")
	cmd := newTestCmd("status")
	require.NoError(t, cmd.Flags().Set("account", "monalisa@github.com"))

	// Then the call succeeds (flag value "monalisa@github.com" is valid and takes precedence)
	err = applyAccountOverride(cmd, cfg)
	require.NoError(t, err)

	// Then the override should resolve to monalisa's token (from the flag), not hubot's (from env)
	token, _ := cfg.Authentication().ActiveToken("github.com")
	require.Equal(t, "test-token", token) // monalisa's token from setupTestConfig
}

func TestApplyAccountOverride_InvalidFormatProducesError(t *testing.T) {
	cfg := setupTestConfig(t, "github.com", "monalisa", "test-token")

	cmd := newTestCmd("status")
	require.NoError(t, cmd.Flags().Set("account", "monalisa"))

	// When we apply with a value missing the @host part
	err := applyAccountOverride(cmd, cfg)

	// Then we get a format error
	require.Error(t, err)
	assert.Contains(t, err.Error(), `invalid account format`)
	assert.Contains(t, err.Error(), `user@host`)
}

func TestApplyAccountOverride_UnknownAccountProducesError(t *testing.T) {
	// Given a config with one logged-in user
	cfg := setupTestConfig(t, "github.com", "monalisa", "test-token")

	cmd := newTestCmd("status")
	require.NoError(t, cmd.Flags().Set("account", "ghost@github.com"))

	// When we apply with an account that doesn't exist
	err := applyAccountOverride(cmd, cfg)

	// Then we get an error that names the missing account and lists available ones
	require.Error(t, err)
	assert.Contains(t, err.Error(), `account "ghost@github.com" not found`)
	assert.Contains(t, err.Error(), "monalisa@github.com")
}

func TestApplyAccountOverride_GHTokenWarnsAndReturnsNil(t *testing.T) {
	// Given a config with a logged-in user
	cfg := setupTestConfig(t, "github.com", "monalisa", "test-token")

	// When GH_TOKEN is also set
	t.Setenv("GH_TOKEN", "env-token")
	cmd := newTestCmd("status")
	require.NoError(t, cmd.Flags().Set("account", "monalisa@github.com"))

	// Then applyAccountOverride returns nil (no error) and skips the override
	err := applyAccountOverride(cmd, cfg)
	require.NoError(t, err)
}

func TestApplyAccountOverride_AuthMutationCommandWarnsAndReturnsNil(t *testing.T) {
	// Given a config with a logged-in user
	cfg := setupTestConfig(t, "github.com", "monalisa", "test-token")

	// When account is set for a mutation command (e.g. "login" under "auth")
	authCmd := &cobra.Command{Use: "auth"}
	loginCmd := newTestCmd("login")
	authCmd.AddCommand(loginCmd)
	require.NoError(t, loginCmd.Flags().Set("account", "monalisa@github.com"))

	// Then applyAccountOverride returns nil (no error) but does NOT set override
	err := applyAccountOverride(loginCmd, cfg)
	require.NoError(t, err)
}

func TestApplyAccountOverride_NoFlagNoEnvIsNoop(t *testing.T) {
	// Given a config (no account flag, no env var)
	cfg := setupTestConfig(t, "github.com", "monalisa", "test-token")
	cmd := newTestCmd("status")

	// When we apply with nothing set
	err := applyAccountOverride(cmd, cfg)

	// Then it's a no-op with no error
	require.NoError(t, err)
}
