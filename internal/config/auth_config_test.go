package config

import (
	"testing"

	ghConfig "github.com/cli/go-gh/v2/pkg/config"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

func newTestAuthConfig() *AuthConfig {
	return &AuthConfig{
		cfg: ghConfig.ReadFromString(""),
	}
}

func TestTokenFromKeyring(t *testing.T) {
	// Given a keyring that contains a token for a host
	keyring.MockInit()
	keyring.Set(keyringServiceName("github.com"), "", "test-token")

	// When we get the token from the auth config
	authCfg := newTestAuthConfig()
	token, err := authCfg.TokenFromKeyring("github.com")

	// Then it returns successfully with the correct token
	require.NoError(t, err)
	require.Equal(t, "test-token", token)
}

func TestTokenFromKeyringNonExistent(t *testing.T) {
	// Given a keyring that doesn't contain any tokens
	keyring.MockInit()

	// When we try to get a token from the auth config
	authCfg := newTestAuthConfig()
	_, err := authCfg.TokenFromKeyring("github.com")

	// Then it returns failure bubbling the ErrNotFound
	require.ErrorIs(t, err, keyring.ErrNotFound)
}

func TestNoUserInAuthConfig(t *testing.T) {
	// Given a host configuration without a user
	authCfg := newTestAuthConfig()

	// When we get the user
	_, err := authCfg.User("github.com")

	// Then it returns failure, bubbling the KeyNotFoundError
	var keyNotFoundError *ghConfig.KeyNotFoundError
	require.ErrorAs(t, err, &keyNotFoundError)
}

func TestUserInAuthConfig(t *testing.T) {
	// Given an a host configuration with a user
	authCfg := newTestAuthConfig()
	authCfg.cfg.Set([]string{hosts, "github.com", "user"}, "test-user")

	// When we get the user
	user, err := authCfg.User("github.com")

	// Then it returns success with the correct user
	require.NoError(t, err)
	require.Equal(t, "test-user", user)
}

func TestNoGitProtocolInAuthConfig(t *testing.T) {
	// Given a host configuration without a git protocol
	authCfg := newTestAuthConfig()

	// When we get the git protocol
	gitProtocol, err := authCfg.GitProtocol("github.com")

	// Then it returns success, using the default
	require.NoError(t, err)
	require.Equal(t, "https", gitProtocol)
}

func TestGitProtocolInAuthConfig(t *testing.T) {
	// Given an a host configuration with a git protocol
	authCfg := newTestAuthConfig()
	authCfg.cfg.Set([]string{hosts, "github.com", "git_protocol"}, "ssh")

	// When we get the git protocol
	gitProtocol, err := authCfg.GitProtocol("github.com")

	// Then it returns success with the correct git protocol
	require.NoError(t, err)
	require.Equal(t, "ssh", gitProtocol)
}
