package config

import (
	"errors"
	"testing"

	"github.com/cli/cli/v2/internal/config/migration"
	"github.com/cli/cli/v2/internal/keyring"
	ghConfig "github.com/cli/go-gh/v2/pkg/config"
	"github.com/stretchr/testify/require"
)

// Note that NewIsolatedTestConfig sets up a Mock keyring as well
func newTestAuthConfig(t *testing.T) *AuthConfig {
	cfg, _ := NewIsolatedTestConfig(t, "")
	return &AuthConfig{cfg: cfg.cfg}
}

func TestTokenFromKeyring(t *testing.T) {
	// Given a keyring that contains a token for a host
	authCfg := newTestAuthConfig(t)
	require.NoError(t, keyring.Set(keyringServiceName("github.com"), "", "test-token"))

	// When we get the token from the auth config
	token, err := authCfg.TokenFromKeyring("github.com")

	// Then it returns successfully with the correct token
	require.NoError(t, err)
	require.Equal(t, "test-token", token)
}

func TestTokenFromKeyringForUser(t *testing.T) {
	// Given a keyring that contains a token for a host with a specific user
	authCfg := newTestAuthConfig(t)
	require.NoError(t, keyring.Set(keyringServiceName("github.com"), "test-user", "test-token"))

	// When we get the token from the auth config
	token, err := authCfg.TokenFromKeyringForUser("github.com", "test-user")

	// Then it returns successfully with the correct token
	require.NoError(t, err)
	require.Equal(t, "test-token", token)
}

func TestTokenFromKeyringForUserErrorsIfUsernameIsBlank(t *testing.T) {
	authCfg := newTestAuthConfig(t)

	// When we get the token from the keyring for an empty username
	_, err := authCfg.TokenFromKeyringForUser("github.com", "")

	// Then it returns an error
	require.ErrorContains(t, err, "username cannot be blank")
}

func TestHasActiveToken(t *testing.T) {
	// Given the user has logged in for a host
	authCfg := newTestAuthConfig(t)
	_, err := authCfg.Login("github.com", "test-user", "test-token", "", false)
	require.NoError(t, err)

	// When we check if that host has an active token
	hasActiveToken := authCfg.HasActiveToken("github.com")

	// Then there is an active token
	require.True(t, hasActiveToken, "expected there to be an active token")
}

func TestHasNoActiveToken(t *testing.T) {
	// Given there are no users logged in for a host
	authCfg := newTestAuthConfig(t)

	// When we check if any host has an active token
	hasActiveToken := authCfg.HasActiveToken("github.com")

	// Then there is no active token
	require.False(t, hasActiveToken, "expected there to be no active token")
}

func TestActiveTokenWithErrorFallsBackWhenActiveUserIsBlank(t *testing.T) {
	authCfg := newTestAuthConfig(t)
	authCfg.cfg.Set([]string{hostsKey, "github.com", userKey}, "")
	require.NoError(t, keyring.Set(keyringServiceName("github.com"), "", "test-token"))

	token, source, err := authCfg.ActiveTokenWithError("github.com")

	require.NoError(t, err)
	require.Equal(t, "test-token", token)
	require.Equal(t, "keyring", source)
}

func TestActiveTokenWithErrorSurfacesKeyringFailure(t *testing.T) {
	// Given a host with a configured user whose token lives in the keyring
	authCfg := newTestAuthConfig(t)
	_, err := authCfg.Login("github.com", "test-user", "test-token", "", true)
	require.NoError(t, err)

	// And a keyring that subsequently fails on every access (simulating a
	// macOS Keychain access timeout or other non-ErrNotFound failure)
	keyringErr := errors.New("simulated keyring failure")
	keyring.MockInitWithError(keyringErr)

	// When we resolve the active token via ActiveTokenWithError
	token, _, err := authCfg.ActiveTokenWithError("github.com")

	// Then the failure is surfaced rather than silently producing an empty
	// token, so callers can distinguish "no token here" from "keyring is
	// inaccessible" and avoid sending unauthenticated requests.
	require.Empty(t, token)
	require.Error(t, err)
	require.ErrorIs(t, err, keyringErr)
	require.ErrorContains(t, err, "github.com")
}

func TestActiveTokenWithErrorSurfacesUserScopedFailureWhenUnkeyedFallbackIsAbsent(t *testing.T) {
	// Given a host with a configured user whose user-scoped keyring lookup
	// fails with a real keyring access error (not ErrNotFound), while the
	// legacy unkeyed fallback slot is genuinely empty (ErrNotFound).
	//
	// This is the regression case for the silent-failure bug: the fallback's
	// "not found" must not be allowed to mask the user-scoped lookup's real
	// failure, otherwise AddAuthTokenHeader will proceed unauthenticated.
	authCfg := newTestAuthConfig(t)
	_, err := authCfg.Login("github.com", "test-user", "test-token", "", true)
	require.NoError(t, err)

	keyringErr := errors.New("simulated user-scoped keyring failure")
	keyring.MockGetOverride(func(_, user string) (string, error) {
		if user == "" {
			return "", keyring.ErrNotFound
		}
		return "", keyringErr
	})
	t.Cleanup(func() { keyring.MockGetOverride(nil) })

	// When we resolve the active token via ActiveTokenWithError
	token, _, err := authCfg.ActiveTokenWithError("github.com")

	// Then the user-scoped failure is surfaced rather than being masked by
	// the fallback's ErrNotFound.
	require.Empty(t, token)
	require.Error(t, err)
	require.ErrorIs(t, err, keyringErr)
	require.ErrorContains(t, err, "github.com")
}

func TestActiveTokenWithErrorPrefersFallbackErrorWhenBothLookupsFail(t *testing.T) {
	// Given a host with a configured user where the user-scoped keyring lookup
	// fails with a real error AND the legacy unkeyed fallback also fails with
	// a different real error. This locks in the current precedence: when both
	// lookups produce non-ErrNotFound errors the fallback's error wins, since
	// the swap at config.go only fires when the fallback returns ErrNotFound.
	// Either error is "real" enough to surface; this test is characterization:
	// flipping precedence would silently change which error message users see,
	// so any future change should be deliberate and update this test.
	authCfg := newTestAuthConfig(t)
	_, err := authCfg.Login("github.com", "test-user", "test-token", "", true)
	require.NoError(t, err)

	userScopedErr := errors.New("simulated user-scoped failure")
	unkeyedErr := errors.New("simulated unkeyed failure")
	keyring.MockGetOverride(func(_, user string) (string, error) {
		if user == "" {
			return "", unkeyedErr
		}
		return "", userScopedErr
	})
	t.Cleanup(func() { keyring.MockGetOverride(nil) })

	// When we resolve the active token via ActiveTokenWithError
	token, _, err := authCfg.ActiveTokenWithError("github.com")

	// Then a real failure is surfaced. The fallback's error is the one that
	// reaches the wrapper; the user-scoped error is intentionally not
	// preferred when both are real.
	require.Empty(t, token)
	require.Error(t, err)
	require.ErrorIs(t, err, unkeyedErr)
	require.NotErrorIs(t, err, userScopedErr)
	require.ErrorContains(t, err, "github.com")
}

func TestActiveTokenWithErrorSurfacesUnkeyedFailureWhenUserScopedReturnsNotFound(t *testing.T) {
	// Given a host with a configured user whose user-scoped keyring lookup
	// returns ErrNotFound (legitimately empty) while the legacy unkeyed slot
	// fails with a real keyring access error. The user-scoped ErrNotFound
	// must not be stashed (it's not a real failure), so the unkeyed real
	// error reaches the wrapper.
	authCfg := newTestAuthConfig(t)
	_, err := authCfg.Login("github.com", "test-user", "test-token", "", true)
	require.NoError(t, err)

	unkeyedErr := errors.New("simulated unkeyed failure")
	keyring.MockGetOverride(func(_, user string) (string, error) {
		if user == "" {
			return "", unkeyedErr
		}
		return "", keyring.ErrNotFound
	})
	t.Cleanup(func() { keyring.MockGetOverride(nil) })

	// When we resolve the active token via ActiveTokenWithError
	token, _, err := authCfg.ActiveTokenWithError("github.com")

	// Then the unkeyed real failure is surfaced.
	require.Empty(t, token)
	require.Error(t, err)
	require.ErrorIs(t, err, unkeyedErr)
	require.ErrorContains(t, err, "github.com")
}

func TestActiveTokenSilentlyDiscardsKeyringFailure(t *testing.T) {
	// Given the same scenario as above
	authCfg := newTestAuthConfig(t)
	_, err := authCfg.Login("github.com", "test-user", "test-token", "", true)
	require.NoError(t, err)
	keyring.MockInitWithError(errors.New("simulated keyring failure"))

	// When we resolve the active token via the original ActiveToken
	token, _ := authCfg.ActiveToken("github.com")

	// Then it returns empty without surfacing the error, preserving its
	// historical signature for existing callers that have not migrated to
	// ActiveTokenWithError.
	require.Empty(t, token)
}

func TestActiveTokenWithErrorDoesNotSurfaceErrNotFound(t *testing.T) {
	// Given a fresh config with no users and a clean keyring (ErrNotFound
	// is the legitimate "no token configured for this host" signal)
	authCfg := newTestAuthConfig(t)

	// When we resolve the active token via ActiveTokenWithError
	token, _, err := authCfg.ActiveTokenWithError("github.com")

	// Then the empty token is returned without an error, so callers can
	// proceed anonymously against unconfigured hosts.
	require.Empty(t, token)
	require.NoError(t, err)
}

func TestActiveTokenWithErrorReturnsEnvTokenWithoutKeyring(t *testing.T) {
	// Given an environment-provided token and a keyring that would fail if
	// it were consulted
	t.Setenv("GH_TOKEN", "env-test-token")
	authCfg := newTestAuthConfig(t)
	keyring.MockInitWithError(errors.New("keyring should not be consulted"))

	// When we resolve the active token via ActiveTokenWithError
	token, source, err := authCfg.ActiveTokenWithError("github.com")

	// Then the env-var path short-circuits keyring access entirely and
	// returns successfully without error.
	require.NoError(t, err)
	require.Equal(t, "env-test-token", token)
	require.Equal(t, "GH_TOKEN", source)
}

func TestActiveTokenWithErrorReturnsKeyringToken(t *testing.T) {
	// Given a host with a configured user and a healthy keyring containing
	// that user's token
	authCfg := newTestAuthConfig(t)
	_, err := authCfg.Login("github.com", "test-user", "test-token", "", true)
	require.NoError(t, err)

	// When we resolve the active token via ActiveTokenWithError
	token, source, err := authCfg.ActiveTokenWithError("github.com")

	// Then it returns the keyring-stored token with the "keyring" source
	// label and no error.
	require.NoError(t, err)
	require.Equal(t, "test-token", token)
	require.Equal(t, "keyring", source)
}

func TestTokenStoredInConfig(t *testing.T) {
	// Given the user has logged in insecurely
	authCfg := newTestAuthConfig(t)
	_, err := authCfg.Login("github.com", "test-user", "test-token", "", false)
	require.NoError(t, err)

	// When we get the token
	token, source := authCfg.ActiveToken("github.com")

	// Then the token is successfully fetched
	// and the source is set to oauth_token but this isn't great:
	// https://github.com/cli/go-gh/issues/94
	require.Equal(t, "test-token", token)
	require.Equal(t, oauthTokenKey, source)
}

func TestTokenStoredInEnv(t *testing.T) {
	// When the user is authenticated via env var
	authCfg := newTestAuthConfig(t)
	t.Setenv("GH_TOKEN", "test-token")

	// When we get the token
	token, source := authCfg.ActiveToken("github.com")

	// Then the token is successfully fetched
	// and the source is set to the name of the env var
	require.Equal(t, "test-token", token)
	require.Equal(t, "GH_TOKEN", source)
}

func TestTokenStoredInKeyring(t *testing.T) {
	// When the user has logged in securely
	authCfg := newTestAuthConfig(t)
	_, err := authCfg.Login("github.com", "test-user", "test-token", "", true)
	require.NoError(t, err)

	// When we get the token
	token, source := authCfg.ActiveToken("github.com")

	// Then the token is successfully fetched
	// and the source is set to keyring
	require.Equal(t, "test-token", token)
	require.Equal(t, "keyring", source)
}

func TestTokenFromKeyringNonExistent(t *testing.T) {
	// Given a keyring that doesn't contain any tokens
	authCfg := newTestAuthConfig(t)

	// When we try to get a token from the auth config
	_, err := authCfg.TokenFromKeyring("github.com")

	// Then it returns failure bubbling the ErrNotFound
	require.ErrorContains(t, err, "secret not found in keyring")
}

func TestHasEnvTokenWithoutAnyEnvToken(t *testing.T) {
	// Given we have no env set
	authCfg := newTestAuthConfig(t)

	// When we check if it has an env token
	hasEnvToken := authCfg.HasEnvToken()

	// Then it returns false
	require.False(t, hasEnvToken, "expected not to have env token")
}

func TestHasEnvTokenWithEnvToken(t *testing.T) {
	// Given we have an env token set
	// Note that any valid env var for tokens will do, not just GH_ENTERPRISE_TOKEN
	authCfg := newTestAuthConfig(t)
	t.Setenv("GH_ENTERPRISE_TOKEN", "test-token")

	// When we check if it has an env token
	hasEnvToken := authCfg.HasEnvToken()

	// Then it returns true
	require.True(t, hasEnvToken, "expected to have env token")
}

func TestHasEnvTokenWithNoEnvTokenButAConfigVar(t *testing.T) {
	t.Skip("this test is explicitly breaking some implementation assumptions")

	// Given a token in the config
	authCfg := newTestAuthConfig(t)
	// Using example.com here will cause the token to be returned from the config
	_, err := authCfg.Login("example.com", "test-user", "test-token", "", false)
	require.NoError(t, err)

	// When we check if it has an env token
	hasEnvToken := authCfg.HasEnvToken()

	// Then it SHOULD return false
	require.False(t, hasEnvToken, "expected not to have env token")
}

func TestUserNotLoggedIn(t *testing.T) {
	// Given we have not logged in
	authCfg := newTestAuthConfig(t)

	// When we get the user
	_, err := authCfg.ActiveUser("github.com")

	// Then it returns failure, bubbling the KeyNotFoundError
	var keyNotFoundError *ghConfig.KeyNotFoundError
	require.ErrorAs(t, err, &keyNotFoundError)
}

func TestHostsIncludesEnvVar(t *testing.T) {
	// Given the GH_HOST env var is set
	authCfg := newTestAuthConfig(t)
	t.Setenv("GH_HOST", "ghe.io")

	// When we get the hosts
	hosts := authCfg.Hosts()

	// Then the host in the env var is included
	require.Contains(t, hosts, "ghe.io")
}

func TestDefaultHostFromEnvVar(t *testing.T) {
	// Given the GH_HOST env var is set
	authCfg := newTestAuthConfig(t)
	t.Setenv("GH_HOST", "ghe.io")

	// When we get the DefaultHost
	defaultHost, source := authCfg.DefaultHost()

	// Then the returned host and source are using the env var
	require.Equal(t, "ghe.io", defaultHost)
	require.Equal(t, "GH_HOST", source)
}

func TestDefaultHostNotLoggedIn(t *testing.T) {
	// Given we are not logged in
	authCfg := newTestAuthConfig(t)

	// When we get the DefaultHost
	defaultHost, source := authCfg.DefaultHost()

	// Then the returned host is always github.com
	require.Equal(t, "github.com", defaultHost)
	require.Equal(t, "default", source)
}

func TestDefaultHostLoggedInToOnlyOneHost(t *testing.T) {
	// Given we are logged into one host (not github.com to differentiate from the fallback)
	authCfg := newTestAuthConfig(t)
	_, err := authCfg.Login("ghe.io", "test-user", "test-token", "", false)
	require.NoError(t, err)

	// When we get the DefaultHost
	defaultHost, source := authCfg.DefaultHost()

	// Then the returned host is that logged in host and the source is the hosts config
	require.Equal(t, "ghe.io", defaultHost)
	require.Equal(t, hostsKey, source)
}

func TestLoginSecureStorageUsesKeyring(t *testing.T) {
	// Given a usable keyring
	authCfg := newTestAuthConfig(t)
	host := "github.com"
	user := "test-user"
	token := "test-token"

	// When we login with secure storage
	insecureStorageUsed, err := authCfg.Login(host, user, token, "", true)

	// Then it returns success, notes that insecure storage was not used, and stores the token in the keyring
	require.NoError(t, err)
	require.False(t, insecureStorageUsed, "expected to use secure storage")

	gotToken, err := keyring.Get(keyringServiceName(host), "")
	require.NoError(t, err)
	require.Equal(t, token, gotToken)

	gotToken, err = keyring.Get(keyringServiceName(host), user)
	require.NoError(t, err)
	require.Equal(t, token, gotToken)
}

func TestLoginSecureStorageRemovesOldInsecureConfigToken(t *testing.T) {
	// Given a usable keyring and an oauth token in the config
	authCfg := newTestAuthConfig(t)
	authCfg.cfg.Set([]string{hostsKey, "github.com", oauthTokenKey}, "old-token")

	// When we login with secure storage
	_, err := authCfg.Login("github.com", "test-user", "test-token", "", true)

	// Then it returns success, having also removed the old token from the config
	require.NoError(t, err)
	requireNoKey(t, authCfg.cfg, []string{hostsKey, "github.com", oauthTokenKey})
}

func TestLoginSecureStorageWithErrorFallsbackAndReports(t *testing.T) {
	// Given a keyring that errors
	authCfg := newTestAuthConfig(t)
	keyring.MockInitWithError(errors.New("test-explosion"))

	// When we login with secure storage
	insecureStorageUsed, err := authCfg.Login("github.com", "test-user", "test-token", "", true)

	// Then it returns success, reports that insecure storage was used, and stores the token in the config
	require.NoError(t, err)

	require.True(t, insecureStorageUsed, "expected to use insecure storage")
	requireKeyWithValue(t, authCfg.cfg, []string{hostsKey, "github.com", oauthTokenKey}, "test-token")
}

func TestLoginInsecureStorage(t *testing.T) {
	// Given we are not logged in
	authCfg := newTestAuthConfig(t)

	// When we login with insecure storage
	insecureStorageUsed, err := authCfg.Login("github.com", "test-user", "test-token", "", false)

	// Then it returns success, notes that insecure storage was used, and stores the token in the config
	require.NoError(t, err)

	require.True(t, insecureStorageUsed, "expected to use insecure storage")
	requireKeyWithValue(t, authCfg.cfg, []string{hostsKey, "github.com", oauthTokenKey}, "test-token")
}

func TestLoginSetsUserForProvidedHost(t *testing.T) {
	// Given we are not logged in
	authCfg := newTestAuthConfig(t)

	// When we login
	_, err := authCfg.Login("github.com", "test-user", "test-token", "ssh", false)

	// Then it returns success and the user is set
	require.NoError(t, err)

	user, err := authCfg.ActiveUser("github.com")
	require.NoError(t, err)
	require.Equal(t, "test-user", user)
}

func TestLoginSetsGitProtocolForProvidedHost(t *testing.T) {
	// Given we are logged in
	authCfg := newTestAuthConfig(t)
	_, err := authCfg.Login("github.com", "test-user", "test-token", "ssh", false)
	require.NoError(t, err)

	// When we get the host git protocol
	hostProtocol, err := authCfg.cfg.Get([]string{hostsKey, "github.com", gitProtocolKey})
	require.NoError(t, err)

	// Then it returns the git protocol we provided on login
	require.Equal(t, "ssh", hostProtocol)
}

func TestLoginAddsHostIfNotAlreadyAdded(t *testing.T) {
	// Given we are logged in
	authCfg := newTestAuthConfig(t)
	_, err := authCfg.Login("github.com", "test-user", "test-token", "ssh", false)
	require.NoError(t, err)

	// When we get the hosts
	hosts := authCfg.Hosts()

	// Then it includes our logged in host
	require.Contains(t, hosts, "github.com")
}

// This test mimics the behaviour of logging in with a token, not providing
// a git protocol, and using secure storage.
func TestLoginAddsUserToConfigWithoutGitProtocolAndWithSecureStorage(t *testing.T) {
	// Given we are not logged in
	authCfg := newTestAuthConfig(t)

	// When we log in without git protocol and with secure storage
	_, err := authCfg.Login("github.com", "test-user", "test-token", "", true)
	require.NoError(t, err)

	// Then the username is added under the users config
	users, err := authCfg.cfg.Keys([]string{hostsKey, "github.com", usersKey})
	require.NoError(t, err)
	require.Contains(t, users, "test-user")
}

func TestLogoutRemovesHostAndKeyringToken(t *testing.T) {
	// Given we are logged into a host
	authCfg := newTestAuthConfig(t)
	host := "github.com"
	user := "test-user"
	token := "test-token"

	_, err := authCfg.Login(host, user, token, "ssh", true)
	require.NoError(t, err)

	// When we logout
	err = authCfg.Logout(host, user)

	// Then we return success, and the host and token are removed from the config and keyring
	require.NoError(t, err)

	requireNoKey(t, authCfg.cfg, []string{hostsKey, host})
	_, err = keyring.Get(keyringServiceName(host), "")
	require.ErrorContains(t, err, "secret not found in keyring")
	_, err = keyring.Get(keyringServiceName(host), user)
	require.ErrorContains(t, err, "secret not found in keyring")
}

func TestLogoutOfActiveUserSwitchesUserIfPossible(t *testing.T) {
	// Given we have two accounts logged into a host
	authCfg := newTestAuthConfig(t)
	_, err := authCfg.Login("github.com", "inactive-user", "test-token-1", "ssh", true)
	require.NoError(t, err)

	_, err = authCfg.Login("github.com", "active-user", "test-token-2", "https", true)
	require.NoError(t, err)

	// When we logout of the active user
	err = authCfg.Logout("github.com", "active-user")

	// Then we return success and the inactive user is now active
	require.NoError(t, err)
	activeUser, err := authCfg.ActiveUser("github.com")
	require.NoError(t, err)
	require.Equal(t, "inactive-user", activeUser)

	token, err := authCfg.TokenFromKeyring("github.com")
	require.NoError(t, err)
	require.Equal(t, "test-token-1", token)

	usersForHost := authCfg.UsersForHost("github.com")
	require.NotContains(t, "active-user", usersForHost)
}

func TestLogoutOfInactiveUserDoesNotSwitchUser(t *testing.T) {
	// Given we have two accounts logged into a host
	authCfg := newTestAuthConfig(t)
	_, err := authCfg.Login("github.com", "inactive-user-1", "test-token-1.1", "ssh", true)
	require.NoError(t, err)

	_, err = authCfg.Login("github.com", "inactive-user-2", "test-token-1.2", "ssh", true)
	require.NoError(t, err)

	_, err = authCfg.Login("github.com", "active-user", "test-token-2", "https", true)
	require.NoError(t, err)

	// When we logout of an inactive user
	err = authCfg.Logout("github.com", "inactive-user-1")

	// Then we return success and the active user is still active
	require.NoError(t, err)
	activeUser, err := authCfg.ActiveUser("github.com")
	require.NoError(t, err)
	require.Equal(t, "active-user", activeUser)
}

// Note that I'm not sure this test enforces particularly desirable behaviour
// since it leads users to believe a token has been removed when really
// that might have failed for some reason.
//
// The original intention here is that if the logout fails, the user can't
// really do anything to recover. On the other hand, a user might
// want to rectify this manually, for example if there were on a shared machine.
func TestLogoutIgnoresErrorsFromConfigAndKeyring(t *testing.T) {
	// Given we have keyring that errors, and a config that
	// doesn't even have a hosts key (which would cause Remove to fail)
	keyring.MockInitWithError(errors.New("test-explosion"))
	authCfg := newTestAuthConfig(t)

	// When we logout
	err := authCfg.Logout("github.com", "test-user")

	// Then it returns success anyway, suppressing the errors
	require.NoError(t, err)
}

func TestSwitchUserMakesSecureTokenActive(t *testing.T) {
	// Given we have a user with a secure token
	authCfg := newTestAuthConfig(t)
	_, err := authCfg.Login("github.com", "test-user-1", "test-token-1", "ssh", true)
	require.NoError(t, err)
	_, err = authCfg.Login("github.com", "test-user-2", "test-token-2", "ssh", true)
	require.NoError(t, err)

	// When we switch to that user
	require.NoError(t, authCfg.SwitchUser("github.com", "test-user-1"))

	// Their secure token is now active
	token, err := authCfg.TokenFromKeyring("github.com")
	require.NoError(t, err)
	require.Equal(t, "test-token-1", token)
}

func TestSwitchUserMakesInsecureTokenActive(t *testing.T) {
	// Given we have a user with an insecure token
	authCfg := newTestAuthConfig(t)
	_, err := authCfg.Login("github.com", "test-user-1", "test-token-1", "ssh", false)
	require.NoError(t, err)
	_, err = authCfg.Login("github.com", "test-user-2", "test-token-2", "ssh", false)
	require.NoError(t, err)

	// When we switch to that user
	require.NoError(t, authCfg.SwitchUser("github.com", "test-user-1"))

	// Their insecure token is now active
	token, source := authCfg.ActiveToken("github.com")
	require.Equal(t, "test-token-1", token)
	require.Equal(t, oauthTokenKey, source)
}

func TestSwitchUserUpdatesTheActiveUser(t *testing.T) {
	// Given we have two users logged into a host
	authCfg := newTestAuthConfig(t)
	_, err := authCfg.Login("github.com", "test-user-1", "test-token-1", "ssh", false)
	require.NoError(t, err)
	_, err = authCfg.Login("github.com", "test-user-2", "test-token-2", "ssh", false)
	require.NoError(t, err)

	// When we switch to the other user
	require.NoError(t, authCfg.SwitchUser("github.com", "test-user-1"))

	// Then the active user is updated
	activeUser, err := authCfg.ActiveUser("github.com")
	require.NoError(t, err)
	require.Equal(t, "test-user-1", activeUser)
}

func TestSwitchUserErrorsImmediatelyIfTheActiveTokenComesFromEnvironment(t *testing.T) {
	// Given we have a token in the env
	authCfg := newTestAuthConfig(t)
	t.Setenv("GH_TOKEN", "unimportant-test-value")
	_, err := authCfg.Login("github.com", "test-user-1", "test-token-1", "ssh", true)
	require.NoError(t, err)
	_, err = authCfg.Login("github.com", "test-user-2", "test-token-2", "ssh", true)
	require.NoError(t, err)

	// When we switch to a user
	err = authCfg.SwitchUser("github.com", "test-user-1")

	// Then it errors immediately with an informative message
	require.ErrorContains(t, err, "currently active token for github.com is from GH_TOKEN")
}

func TestSwitchUserErrorsAndRestoresUserAndInsecureConfigUnderFailure(t *testing.T) {
	// Given we have a user but no token can be found (because we deleted them, simulating an error case)
	authCfg := newTestAuthConfig(t)
	_, err := authCfg.Login("github.com", "test-user-1", "test-token-1", "ssh", true)
	require.NoError(t, err)
	_, err = authCfg.Login("github.com", "test-user-2", "test-token-2", "ssh", false)
	require.NoError(t, err)

	require.NoError(t, keyring.Delete(keyringServiceName("github.com"), "test-user-1"))

	// When we switch to the user
	err = authCfg.SwitchUser("github.com", "test-user-1")

	// Then it returns an error
	require.EqualError(t, err, "no token found for test-user-1")

	// And restores the previous state
	activeUser, err := authCfg.ActiveUser("github.com")
	require.NoError(t, err)
	require.Equal(t, "test-user-2", activeUser)

	token, source := authCfg.ActiveToken("github.com")
	require.Equal(t, "test-token-2", token)
	require.Equal(t, "oauth_token", source)
}

func TestSwitchUserErrorsAndRestoresUserAndKeyringUnderFailure(t *testing.T) {
	// Given we have a user but no token can be found (because we deleted them, simulating an error case)
	authCfg := newTestAuthConfig(t)
	_, err := authCfg.Login("github.com", "test-user-1", "test-token-1", "ssh", false)
	require.NoError(t, err)
	_, err = authCfg.Login("github.com", "test-user-2", "test-token-2", "ssh", true)
	require.NoError(t, err)

	require.NoError(t, authCfg.cfg.Remove([]string{hostsKey, "github.com", usersKey, "test-user-1", oauthTokenKey}))

	// When we switch to the user
	err = authCfg.SwitchUser("github.com", "test-user-1")

	// Then it returns an error
	require.EqualError(t, err, "no token found for test-user-1")

	// And restores the previous state
	activeUser, err := authCfg.ActiveUser("github.com")
	require.NoError(t, err)
	require.Equal(t, "test-user-2", activeUser)

	token, source := authCfg.ActiveToken("github.com")
	require.Equal(t, "test-token-2", token)
	require.Equal(t, "keyring", source)
}

func TestSwitchClearsActiveSecureTokenWhenSwitchingToInsecureUser(t *testing.T) {
	// Given we have an active secure token
	authCfg := newTestAuthConfig(t)
	_, err := authCfg.Login("github.com", "test-user-1", "test-token-1", "ssh", false)
	require.NoError(t, err)
	_, err = authCfg.Login("github.com", "test-user-2", "test-token-2", "ssh", true)
	require.NoError(t, err)

	// When we switch to an insecure user
	require.NoError(t, authCfg.SwitchUser("github.com", "test-user-1"))

	// Then the active secure token is cleared
	_, err = authCfg.TokenFromKeyring("github.com")
	require.Error(t, err)
}

func TestSwitchClearsActiveInsecureTokenWhenSwitchingToSecureUser(t *testing.T) {
	// Given we have an active insecure token
	authCfg := newTestAuthConfig(t)
	_, err := authCfg.Login("github.com", "test-user-1", "test-token-1", "ssh", true)
	require.NoError(t, err)
	_, err = authCfg.Login("github.com", "test-user-2", "test-token-2", "ssh", false)
	require.NoError(t, err)

	// When we switch to a secure user
	require.NoError(t, authCfg.SwitchUser("github.com", "test-user-1"))

	// Then the active insecure token is cleared
	requireNoKey(t, authCfg.cfg, []string{hostsKey, "github.com", oauthTokenKey})
}

func TestUsersForHostNoHost(t *testing.T) {
	// Given we have a config with no hosts
	authCfg := newTestAuthConfig(t)

	// When we get the users for a host that doesn't exist
	users := authCfg.UsersForHost("github.com")

	// Then it returns nil
	require.Nil(t, users)
}

func TestUsersForHostWithUsers(t *testing.T) {
	// Given we have a config with a host and users
	authCfg := newTestAuthConfig(t)
	_, err := authCfg.Login("github.com", "test-user-1", "test-token", "ssh", false)
	require.NoError(t, err)
	_, err = authCfg.Login("github.com", "test-user-2", "test-token", "ssh", false)
	require.NoError(t, err)

	// When we get the users for that host
	users := authCfg.UsersForHost("github.com")

	// Then it succeeds and returns the users
	require.Equal(t, []string{"test-user-1", "test-user-2"}, users)
}

func TestTokenForUserSecureLogin(t *testing.T) {
	// Given a user has logged in securely
	authCfg := newTestAuthConfig(t)
	_, err := authCfg.Login("github.com", "test-user-1", "test-token", "ssh", true)
	require.NoError(t, err)

	// When we get the token
	token, source, err := authCfg.TokenForUser("github.com", "test-user-1")

	// Then it returns the token and the source as keyring
	require.NoError(t, err)
	require.Equal(t, "test-token", token)
	require.Equal(t, "keyring", source)
}

func TestTokenForUserInsecureLogin(t *testing.T) {
	// Given a user has logged in insecurely
	authCfg := newTestAuthConfig(t)
	_, err := authCfg.Login("github.com", "test-user-1", "test-token", "ssh", false)
	require.NoError(t, err)

	// When we get the token
	token, source, err := authCfg.TokenForUser("github.com", "test-user-1")

	// Then it returns the token and the source as oauth_token
	require.NoError(t, err)
	require.Equal(t, "test-token", token)
	require.Equal(t, "oauth_token", source)
}

func TestTokenForUserNotFoundErrors(t *testing.T) {
	// Given a user has not logged in
	authCfg := newTestAuthConfig(t)

	// When we get the token
	_, _, err := authCfg.TokenForUser("github.com", "test-user-1")

	// Then it returns an error
	require.EqualError(t, err, "no token found for 'test-user-1'")
}

func requireKeyWithValue(t *testing.T, cfg *ghConfig.Config, keys []string, value string) {
	t.Helper()

	actual, err := cfg.Get(keys)
	require.NoError(t, err)

	require.Equal(t, value, actual)
}

func requireNoKey(t *testing.T, cfg *ghConfig.Config, keys []string) {
	t.Helper()

	_, err := cfg.Get(keys)
	var keyNotFoundError *ghConfig.KeyNotFoundError
	require.ErrorAs(t, err, &keyNotFoundError)
}

// Post migration tests

func TestUserWorksRightAfterMigration(t *testing.T) {
	// Given we have logged in before migration
	authCfg := newTestAuthConfig(t)
	_, err := preMigrationLogin(authCfg, "github.com", "test-user", "test-token", "ssh", false)
	require.NoError(t, err)

	// When we migrate
	var m migration.MultiAccount
	c := cfg{authCfg.cfg}
	require.NoError(t, c.Migrate(m))

	// Then we can still get the user correctly
	user, err := authCfg.ActiveUser("github.com")
	require.NoError(t, err)
	require.Equal(t, "test-user", user)
}

func TestGitProtocolWorksRightAfterMigration(t *testing.T) {
	// Given we have logged in before migration with a non-default git protocol
	authCfg := newTestAuthConfig(t)
	_, err := preMigrationLogin(authCfg, "github.com", "test-user", "test-token", "ssh", false)
	require.NoError(t, err)

	// When we migrate
	var m migration.MultiAccount
	c := cfg{authCfg.cfg}
	require.NoError(t, c.Migrate(m))

	// Then we can still get the git protocol correctly
	gitProtocol, err := authCfg.cfg.Get([]string{hostsKey, "github.com", gitProtocolKey})
	require.NoError(t, err)
	require.Equal(t, "ssh", gitProtocol)
}

func TestHostsWorksRightAfterMigration(t *testing.T) {
	// Given we have logged in before migration
	authCfg := newTestAuthConfig(t)
	_, err := preMigrationLogin(authCfg, "ghe.io", "test-user", "test-token", "ssh", false)
	require.NoError(t, err)

	// When we migrate
	var m migration.MultiAccount
	c := cfg{authCfg.cfg}
	require.NoError(t, c.Migrate(m))

	// Then we can still get the hosts correctly
	hosts := authCfg.Hosts()
	require.Contains(t, hosts, "ghe.io")
}

func TestDefaultHostWorksRightAfterMigration(t *testing.T) {
	// Given we have logged in before migration to an enterprise host
	authCfg := newTestAuthConfig(t)
	_, err := preMigrationLogin(authCfg, "ghe.io", "test-user", "test-token", "ssh", false)
	require.NoError(t, err)

	// When we migrate
	var m migration.MultiAccount
	c := cfg{authCfg.cfg}
	require.NoError(t, c.Migrate(m))

	// Then the default host is still the enterprise host
	defaultHost, source := authCfg.DefaultHost()
	require.Equal(t, "ghe.io", defaultHost)
	require.Equal(t, hostsKey, source)
}

func TestTokenWorksRightAfterMigration(t *testing.T) {
	// Given we have logged in before migration
	authCfg := newTestAuthConfig(t)
	_, err := preMigrationLogin(authCfg, "github.com", "test-user", "test-token", "ssh", false)
	require.NoError(t, err)

	// When we migrate
	var m migration.MultiAccount
	c := cfg{authCfg.cfg}
	require.NoError(t, c.Migrate(m))

	// Then we can still get the token correctly
	token, source := authCfg.ActiveToken("github.com")
	require.Equal(t, "test-token", token)
	require.Equal(t, oauthTokenKey, source)
}

func TestTokenPrioritizesActiveUserToken(t *testing.T) {
	// Given a keyring where the active slot contains the token from a previous user
	authCfg := newTestAuthConfig(t)
	require.NoError(t, keyring.Set(keyringServiceName("github.com"), "", "test-token"))
	require.NoError(t, keyring.Set(keyringServiceName("github.com"), "test-user1", "test-token"))
	require.NoError(t, keyring.Set(keyringServiceName("github.com"), "test-user2", "test-token2"))

	// When no active user is set
	authCfg.cfg.Remove([]string{hostsKey, "github.com", userKey})

	// And get the token from the auth config
	token, source := authCfg.ActiveToken("github.com")

	// Then it returns the token from the keyring active slot
	require.Equal(t, "keyring", source)
	require.Equal(t, "test-token", token)

	// When we set the active user to test-user1
	authCfg.cfg.Set([]string{hostsKey, "github.com", userKey}, "test-user1")

	// And get the token from the auth config
	token, source = authCfg.ActiveToken("github.com")

	// Then it returns the token from the active user entry in the keyring
	require.Equal(t, "keyring", source)
	require.Equal(t, "test-token", token)

	// When we set the active user to test-user2
	authCfg.cfg.Set([]string{hostsKey, "github.com", userKey}, "test-user2")

	// And get the token from the auth config
	token, source = authCfg.ActiveToken("github.com")

	// Then it returns the token from the active user entry in the keyring
	require.Equal(t, "keyring", source)
	require.Equal(t, "test-token2", token)
}

func TestTokenWithActiveUserNotInKeyringFallsBackToBlank(t *testing.T) {
	// Given a keyring that contains a token for a host
	authCfg := newTestAuthConfig(t)
	require.NoError(t, keyring.Set(keyringServiceName("github.com"), "", "test-token"))
	require.NoError(t, keyring.Set(keyringServiceName("github.com"), "test-user1", "test-token1"))
	require.NoError(t, keyring.Set(keyringServiceName("github.com"), "test-user2", "test-token2"))

	// When we set the active user to test-user3
	authCfg.cfg.Set([]string{hostsKey, "github.com", userKey}, "test-user3")

	// And get the token from the auth config
	token, source := authCfg.ActiveToken("github.com")

	// Then it returns successfully with the fallback token
	require.Equal(t, "keyring", source)
	require.Equal(t, "test-token", token)
}

func TestLogoutRightAfterMigrationRemovesHost(t *testing.T) {
	// Given we have logged in before migration
	authCfg := newTestAuthConfig(t)
	host := "github.com"
	user := "test-user"
	token := "test-token"

	_, err := preMigrationLogin(authCfg, host, user, token, "ssh", false)
	require.NoError(t, err)

	// When we migrate and logout
	var m migration.MultiAccount
	c := cfg{authCfg.cfg}
	require.NoError(t, c.Migrate(m))

	require.NoError(t, authCfg.Logout(host, user))

	// Then the host is removed from the config
	requireNoKey(t, authCfg.cfg, []string{hostsKey, "github.com"})
}

func TestLoginInsecurePostMigrationUsesConfigForToken(t *testing.T) {
	// Given we have not logged in
	authCfg := newTestAuthConfig(t)

	// When we migrate and login with insecure storage
	var m migration.MultiAccount
	c := cfg{authCfg.cfg}
	require.NoError(t, c.Migrate(m))

	insecureStorageUsed, err := authCfg.Login("github.com", "test-user", "test-token", "", false)

	// Then it returns success, notes that insecure storage was used, and stores the token in the config
	// both under the host and under the user
	require.NoError(t, err)

	require.True(t, insecureStorageUsed, "expected to use insecure storage")
	requireKeyWithValue(t, authCfg.cfg, []string{hostsKey, "github.com", oauthTokenKey}, "test-token")
	requireKeyWithValue(t, authCfg.cfg, []string{hostsKey, "github.com", usersKey, "test-user", oauthTokenKey}, "test-token")
}

func TestLoginPostMigrationSetsGitProtocol(t *testing.T) {
	// Given we have logged in after migration
	authCfg := newTestAuthConfig(t)

	var m migration.MultiAccount
	c := cfg{authCfg.cfg}
	require.NoError(t, c.Migrate(m))

	_, err := authCfg.Login("github.com", "test-user", "test-token", "ssh", false)
	require.NoError(t, err)

	// When we get the host git protocol
	hostProtocol, err := authCfg.cfg.Get([]string{hostsKey, "github.com", gitProtocolKey})
	require.NoError(t, err)

	// Then it returns the git protocol we provided on login
	require.Equal(t, "ssh", hostProtocol)
}

func TestLoginPostMigrationSetsUser(t *testing.T) {
	// Given we have logged in after migration
	authCfg := newTestAuthConfig(t)

	var m migration.MultiAccount
	c := cfg{authCfg.cfg}
	require.NoError(t, c.Migrate(m))

	_, err := authCfg.Login("github.com", "test-user", "test-token", "ssh", false)
	require.NoError(t, err)

	// When we get the user
	user, err := authCfg.ActiveUser("github.com")

	// Then it returns success and the user we provided on login
	require.NoError(t, err)
	require.Equal(t, "test-user", user)
}

func TestLoginSecurePostMigrationRemovesTokenFromConfig(t *testing.T) {
	// Given we have logged in insecurely
	authCfg := newTestAuthConfig(t)
	_, err := preMigrationLogin(authCfg, "github.com", "test-user", "test-token", "", false)
	require.NoError(t, err)

	// When we migrate and login again with secure storage
	var m migration.MultiAccount
	c := cfg{authCfg.cfg}
	require.NoError(t, c.Migrate(m))

	_, err = authCfg.Login("github.com", "test-user", "test-token", "", true)

	// Then it returns success, having removed the old insecure oauth token entry
	require.NoError(t, err)
	requireNoKey(t, authCfg.cfg, []string{hostsKey, "github.com", oauthTokenKey})
	requireNoKey(t, authCfg.cfg, []string{hostsKey, "github.com", usersKey, "test-user", oauthTokenKey})
}

// Copied and pasted directly from the trunk branch before doing any work on
// login, plus the addition of AuthConfig as the first arg since it is a method
// receiver in the real implementation.
func preMigrationLogin(c *AuthConfig, hostname, username, token, gitProtocol string, secureStorage bool) (bool, error) {
	var setErr error
	if secureStorage {
		if setErr = keyring.Set(keyringServiceName(hostname), "", token); setErr == nil {
			// Clean up the previous oauth_token from the config file.
			_ = c.cfg.Remove([]string{hostsKey, hostname, oauthTokenKey})
		}
	}
	insecureStorageUsed := false
	if !secureStorage || setErr != nil {
		c.cfg.Set([]string{hostsKey, hostname, oauthTokenKey}, token)
		insecureStorageUsed = true
	}

	c.cfg.Set([]string{hostsKey, hostname, userKey}, username)

	if gitProtocol != "" {
		c.cfg.Set([]string{hostsKey, hostname, gitProtocolKey}, gitProtocol)
	}
	return insecureStorageUsed, ghConfig.Write(c.cfg)
}
