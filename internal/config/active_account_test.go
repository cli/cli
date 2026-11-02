package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const activeAccountFixture = `
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

func TestActiveUserAndCredentialShareInvocationSelection(t *testing.T) {
	// Given saved defaults on two hosts and an environment token on GitHub
	cfg, _ := NewIsolatedTestConfig(t, activeAccountFixture)
	t.Setenv("GH_TOKEN", "unknown-environment-account")
	auth := cfg.Authentication()

	// When selecting Bob for this invocation on GitHub
	auth.SetAccountOverride("github.com", "bob")

	// Then effective username and credential agree without changing stored defaults or another host
	user, err := auth.ActiveUser("github.com")
	require.NoError(t, err)
	assert.Equal(t, "bob", user)
	cred, _, err := auth.ActiveTokenWithRefresh("github.com")
	require.NoError(t, err)
	assert.Equal(t, "bob-token", cred.Token)
	stored, err := auth.ConfiguredUser("github.com")
	require.NoError(t, err)
	assert.Equal(t, "alice", stored)
	user, err = auth.ActiveUser("enterprise.example")
	require.NoError(t, err)
	assert.Equal(t, "carol", user)
	assert.Equal(t, []string{"GH_IDENTITY_HOST=github.com", "GH_IDENTITY_ACCOUNT=bob"}, auth.AuthenticationEnvironment())
}

func TestEnvironmentTokenHasNoAssumedUsername(t *testing.T) {
	// Given a saved user and an environment credential of unknown ownership
	cfg, _ := NewIsolatedTestConfig(t, activeAccountFixture)
	t.Setenv("GH_TOKEN", "unknown-environment-account")

	// When resolving effective authentication
	user, err := cfg.Authentication().ActiveUser("github.com")

	// Then the saved default is not presented as the environment token's owner
	require.NoError(t, err)
	assert.Empty(t, user)
	assert.Equal(t, "unknown-environment-account", cfg.Authentication().ActiveToken("github.com").Token)
}

func TestActiveAccountSelectionSurvivesSubprocessEnvironment(t *testing.T) {
	// Given a selection forwarded to a credential-helper subprocess
	cfg, _ := NewIsolatedTestConfig(t, activeAccountFixture)
	t.Setenv("GH_IDENTITY_HOST", "github.com")
	t.Setenv("GH_IDENTITY_ACCOUNT", "bob")

	// When the subprocess uses ordinary authentication lookups
	user, err := cfg.Authentication().ActiveUser("github.com")

	// Then the selected account supplies both username and token
	require.NoError(t, err)
	assert.Equal(t, "bob", user)
	cred, _, err := cfg.Authentication().ActiveTokenWithRefresh("github.com")
	require.NoError(t, err)
	assert.Equal(t, "bob-token", cred.Token)
}

func TestMissingSelectedAccountDoesNotFallBack(t *testing.T) {
	// Given a child process selecting an account absent from its saved configuration
	cfg, _ := NewIsolatedTestConfig(t, activeAccountFixture)
	t.Setenv("GH_IDENTITY_HOST", "github.com")
	t.Setenv("GH_IDENTITY_ACCOUNT", "missing")

	// When resolving effective credentials
	cred, _, err := cfg.Authentication().ActiveTokenWithRefresh("github.com")

	// Then neither the persisted default nor environment authentication is substituted
	require.ErrorContains(t, err, `account "missing"`)
	assert.Empty(t, cred.Token)
}

func TestGitCredentialExplicitSelectionWinsOverRequestedUsername(t *testing.T) {
	// Given Git requesting Alice while this invocation explicitly selects Bob
	cfg, _ := NewIsolatedTestConfig(t, activeAccountFixture)
	t.Setenv("GH_IDENTITY_HOST", "github.com")
	t.Setenv("GH_IDENTITY_ACCOUNT", "bob")

	// When resolving the credential request for its destination
	user, cred, _, err := cfg.Authentication().ResolveGitCredential("github.com", "alice", "")

	// Then Bob supplies both fields, without changing the saved default
	require.NoError(t, err)
	assert.Equal(t, "bob", user)
	assert.Equal(t, "bob-token", cred.Token)
	stored, err := cfg.Authentication().ConfiguredUser("github.com")
	require.NoError(t, err)
	assert.Equal(t, "alice", stored)
}

func TestGitCredentialDefaultRejectsRequestedUsernameMismatch(t *testing.T) {
	// Given Alice is the saved default without an invocation selection
	cfg, _ := NewIsolatedTestConfig(t, activeAccountFixture)

	// When Git requests credentials for Bob
	_, cred, _, err := cfg.Authentication().ResolveGitCredential("github.com", "bob", "")

	// Then Alice's credentials are not returned for the conflicting username
	require.NoError(t, err)
	assert.Empty(t, cred.Token)
}

func TestGitCredentialEnvironmentTokenDoesNotAssumeSavedUser(t *testing.T) {
	// Given an environment token of unknown ownership alongside a saved default
	cfg, _ := NewIsolatedTestConfig(t, activeAccountFixture)
	t.Setenv("GH_TOKEN", "unknown-owner-token")

	// When Git requests a username
	user, cred, _, err := cfg.Authentication().ResolveGitCredential("github.com", "some-user", "")

	// Then the helper uses the generic token username, not the saved default
	require.NoError(t, err)
	assert.Equal(t, "x-access-token", user)
	assert.Equal(t, "unknown-owner-token", cred.Token)
}

func TestGitCredentialSelectionDoesNotOverrideAnotherHost(t *testing.T) {
	// Given an explicit selection on GitHub and a different default on an enterprise host
	cfg, _ := NewIsolatedTestConfig(t, activeAccountFixture)
	t.Setenv("GH_IDENTITY_HOST", "github.com")
	t.Setenv("GH_IDENTITY_ACCOUNT", "bob")

	// When Git requests Bob from the enterprise host
	_, cred, _, err := cfg.Authentication().ResolveGitCredential("enterprise.example", "bob", "")

	// Then the GitHub selection cannot bypass the other host's username constraint
	require.NoError(t, err)
	assert.Empty(t, cred.Token)
}
