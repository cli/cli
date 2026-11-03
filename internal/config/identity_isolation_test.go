//go:build ghdebug

package config

import (
	"testing"

	"github.com/cli/cli/v2/internal/keyring"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsolatedIdentityWorkflowNeverUsesOrDeletesOperatorToken(t *testing.T) {
	// Given an operator token in a fake keyring and a separate empty configuration
	cfg, _ := NewIsolatedTestConfig(t, "")
	t.Setenv("GH_INTERNAL_DISABLE_KEYRING", "false")
	require.NoError(t, keyring.Set("gh:github.com", "alice", "operator-token"))
	require.NoError(t, keyring.Set("gh:github.com", "", "operator-active-token"))
	t.Setenv("GH_INTERNAL_DISABLE_KEYRING", "true")

	// When a scenario signs in with its own token and logs out
	_, err := cfg.Authentication().Login("github.com", "alice", "scenario-token", "https", false)
	require.NoError(t, err)
	assert.Equal(t, "scenario-token", cfg.Authentication().ActiveToken("github.com").Token)
	require.NoError(t, cfg.Authentication().Logout("github.com", "alice"))

	// Then the operator's per-account and active credentials survive unchanged
	t.Setenv("GH_INTERNAL_DISABLE_KEYRING", "false")
	account, err := keyring.Get("gh:github.com", "alice")
	require.NoError(t, err)
	active, err := keyring.Get("gh:github.com", "")
	require.NoError(t, err)
	assert.Equal(t, "operator-token", account)
	assert.Equal(t, "operator-active-token", active)
}
