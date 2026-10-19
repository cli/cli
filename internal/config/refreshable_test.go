package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cli/cli/v2/internal/gh"
	ghmock "github.com/cli/cli/v2/internal/gh/mock"
	"github.com/cli/cli/v2/internal/keyring"
	"github.com/stretchr/testify/require"
)

func setRefreshableConfigField(t *testing.T, authCfg *AuthConfig, hostname, username, leaf, value string) {
	t.Helper()
	authCfg.cfg.Set(append(refreshablePath(hostname, username), leaf), value)
}

func setRefreshableKeyring(t *testing.T, hostname, username string, credential gh.Credential) {
	t.Helper()
	data, err := json.Marshal(recordFromCredential(credential))
	require.NoError(t, err)
	require.NoError(t, keyring.Set(refreshableServiceName(hostname), username, string(data)))
}

func TestRefreshableForUser(t *testing.T) {
	expires := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	refreshExpires := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Second)

	tests := []struct {
		name           string
		setup          func(t *testing.T, authCfg *AuthConfig)
		wantCredential gh.Credential
		wantSecure     bool
		wantErr        bool
		wantErrIs      error
	}{
		{
			name: "from config",
			setup: func(t *testing.T, authCfg *AuthConfig) {
				setRefreshableConfigField(t, authCfg, "github.com", "test-user", "access_token", "access")
				setRefreshableConfigField(t, authCfg, "github.com", "test-user", "refresh_token", "refresh")
			},
			wantCredential: gh.Credential{Source: "refreshable_oauth_token", Token: "access", RefreshToken: "refresh"},
			wantSecure:     false,
		},
		{
			name: "from config parses expiries",
			setup: func(t *testing.T, authCfg *AuthConfig) {
				setRefreshableConfigField(t, authCfg, "github.com", "test-user", "access_token", "access")
				setRefreshableConfigField(t, authCfg, "github.com", "test-user", "refresh_token", "refresh")
				setRefreshableConfigField(t, authCfg, "github.com", "test-user", "expires_in", "3600")
				setRefreshableConfigField(t, authCfg, "github.com", "test-user", "expires_at", expires.Format(time.RFC3339Nano))
				setRefreshableConfigField(t, authCfg, "github.com", "test-user", "refresh_token_expires_in", "86400")
				setRefreshableConfigField(t, authCfg, "github.com", "test-user", "refresh_token_expires_at", refreshExpires.Format(time.RFC3339Nano))
			},
			wantCredential: gh.Credential{
				Source:                "refreshable_oauth_token",
				Token:                 "access",
				RefreshToken:          "refresh",
				ExpiresIn:             3600,
				ExpiresAt:             &expires,
				RefreshTokenExpiresIn: 86400,
				RefreshTokenExpiresAt: &refreshExpires,
			},
			wantSecure: false,
		},
		{
			name: "from config invalid expiry timestamp",
			setup: func(t *testing.T, authCfg *AuthConfig) {
				setRefreshableConfigField(t, authCfg, "github.com", "test-user", "access_token", "access")
				setRefreshableConfigField(t, authCfg, "github.com", "test-user", "refresh_token", "refresh")
				setRefreshableConfigField(t, authCfg, "github.com", "test-user", "expires_at", "not-a-timestamp")
			},
			wantErr: true,
		},
		{
			name: "from config invalid expires_in",
			setup: func(t *testing.T, authCfg *AuthConfig) {
				setRefreshableConfigField(t, authCfg, "github.com", "test-user", "access_token", "access")
				setRefreshableConfigField(t, authCfg, "github.com", "test-user", "refresh_token", "refresh")
				setRefreshableConfigField(t, authCfg, "github.com", "test-user", "expires_in", "not-a-number")
			},
			wantErr: true,
		},
		{
			name: "from keyring",
			setup: func(t *testing.T, authCfg *AuthConfig) {
				setRefreshableKeyring(t, "github.com", "test-user", gh.Credential{
					Token:                 "access",
					RefreshToken:          "refresh",
					ExpiresIn:             3600,
					RefreshTokenExpiresIn: 86400,
				})
			},
			wantCredential: gh.Credential{
				Source:                "keyring",
				Token:                 "access",
				RefreshToken:          "refresh",
				ExpiresIn:             3600,
				RefreshTokenExpiresIn: 86400,
			},
			wantSecure: true,
		},
		{
			name: "from keyring invalid json",
			setup: func(t *testing.T, authCfg *AuthConfig) {
				require.NoError(t, keyring.Set(refreshableServiceName("github.com"), "test-user", "not-json"))
			},
			wantErr: true,
		},
		{
			name:      "not found",
			setup:     func(t *testing.T, authCfg *AuthConfig) {},
			wantErrIs: errRefreshableNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authCfg := newTestAuthConfig(t)
			tt.setup(t, authCfg)

			credential, secure, err := authCfg.refreshableForUser("github.com", "test-user")

			if tt.wantErrIs != nil {
				require.ErrorIs(t, err, tt.wantErrIs)
				return
			}
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantSecure, secure)
			require.Equal(t, tt.wantCredential, credential)
		})
	}
}

func TestTokenForUserResolution(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(t *testing.T, authCfg *AuthConfig)
		wantToken  string
		wantSource string
	}{
		{
			name: "falls back to refreshable config",
			setup: func(t *testing.T, authCfg *AuthConfig) {
				setRefreshableConfigField(t, authCfg, "github.com", "test-user", "access_token", "access")
				setRefreshableConfigField(t, authCfg, "github.com", "test-user", "refresh_token", "refresh")
			},
			wantToken:  "access",
			wantSource: "refreshable_oauth_token",
		},
		{
			name: "falls back to refreshable keyring",
			setup: func(t *testing.T, authCfg *AuthConfig) {
				setRefreshableKeyring(t, "github.com", "test-user", gh.Credential{Token: "access", RefreshToken: "refresh"})
			},
			wantToken:  "access",
			wantSource: "keyring",
		},
		{
			// This state should not occur in practice: logging in with a non-refreshable token deletes any
			// refreshable record for that account, and logging in with a refreshable token deletes the non-expiring
			// tokens. The case documents the resolution precedence regardless: the non-expiring config token is probed
			// last, so the managed refreshable credential still wins over it, matching setActiveCredential.
			name: "prefers refreshable over non-expiring config token",
			setup: func(t *testing.T, authCfg *AuthConfig) {
				authCfg.cfg.Set([]string{hostsKey, "github.com", usersKey, "test-user", oauthTokenKey}, "plain-token")
				setRefreshableConfigField(t, authCfg, "github.com", "test-user", "access_token", "access")
				setRefreshableConfigField(t, authCfg, "github.com", "test-user", "refresh_token", "refresh")
			},
			wantToken:  "access",
			wantSource: "refreshable_oauth_token",
		},
		{
			// Also a mixed state the write paths keep unreachable; documents that the non-expiring keyring token is
			// probed before the refreshable credential, so it wins here, matching setActiveCredential.
			name: "prefers non-expiring keyring over refreshable",
			setup: func(t *testing.T, authCfg *AuthConfig) {
				require.NoError(t, keyring.Set(keyringServiceName("github.com"), "test-user", "plain-token"))
				setRefreshableConfigField(t, authCfg, "github.com", "test-user", "access_token", "access")
				setRefreshableConfigField(t, authCfg, "github.com", "test-user", "refresh_token", "refresh")
			},
			wantToken:  "plain-token",
			wantSource: "keyring",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authCfg := newTestAuthConfig(t)
			tt.setup(t, authCfg)

			cred, err := authCfg.TokenForUser("github.com", "test-user")

			require.NoError(t, err)
			require.Equal(t, tt.wantToken, cred.Token)
			require.Equal(t, tt.wantSource, cred.Source)
		})
	}
}

func TestValidateRefreshableCredential(t *testing.T) {
	tests := []struct {
		name    string
		cred    gh.Credential
		wantErr bool
	}{
		{
			name:    "missing access token",
			cred:    gh.Credential{RefreshToken: "refresh"},
			wantErr: true,
		},
		{
			name:    "missing refresh token",
			cred:    gh.Credential{Token: "access"},
			wantErr: true,
		},
		{
			name:    "valid",
			cred:    gh.Credential{Token: "access", RefreshToken: "refresh"},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRefreshableCredential(tt.cred)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestSetActiveCredential(t *testing.T) {
	tests := []struct {
		name         string
		setup        func(t *testing.T, authCfg *AuthConfig)
		wantFound    bool
		assertActive func(t *testing.T, authCfg *AuthConfig)
	}{
		{
			// This mixed state should not occur in practice: the write paths keep the two credential forms mutually
			// exclusive, and loginRefreshable verifies removal of any non-expiring token before storing a refreshable
			// one. The case documents the resolution precedence regardless: the non-expiring keyring token is probed
			// first, so it is activated ahead of the refreshable credential, matching TokenForUser so activation and
			// resolution agree.
			name: "prefers non-expiring keyring over refreshable",
			setup: func(t *testing.T, authCfg *AuthConfig) {
				require.NoError(t, keyring.Set(keyringServiceName("github.com"), "test-user", "plain-token"))
				setRefreshableConfigField(t, authCfg, "github.com", "test-user", "access_token", "access")
				setRefreshableConfigField(t, authCfg, "github.com", "test-user", "refresh_token", "refresh")
			},
			wantFound: true,
			assertActive: func(t *testing.T, authCfg *AuthConfig) {
				// The non-expiring keyring token is moved into the host active slot ahead of the refreshable credential.
				got, err := keyring.Get(keyringServiceName("github.com"), "")
				require.NoError(t, err)
				require.Equal(t, "plain-token", got)
			},
		},
		{
			name: "falls back to non-expiring keyring token",
			setup: func(t *testing.T, authCfg *AuthConfig) {
				require.NoError(t, keyring.Set(keyringServiceName("github.com"), "test-user", "plain-token"))
			},
			wantFound: true,
			assertActive: func(t *testing.T, authCfg *AuthConfig) {
				got, err := keyring.Get(keyringServiceName("github.com"), "")
				require.NoError(t, err)
				require.Equal(t, "plain-token", got)
			},
		},
		{
			name:      "not found",
			setup:     func(t *testing.T, authCfg *AuthConfig) {},
			wantFound: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authCfg := newTestAuthConfig(t)
			tt.setup(t, authCfg)

			found, err := authCfg.setActiveCredential("github.com", "test-user")

			require.NoError(t, err)
			require.Equal(t, tt.wantFound, found)
			if tt.assertActive != nil {
				tt.assertActive(t, authCfg)
			}
		})
	}
}

func TestActivateUserResolvesRefreshableCredential(t *testing.T) {
	authCfg := newTestAuthConfig(t)
	setRefreshableConfigField(t, authCfg, "github.com", "test-user", "access_token", "access")
	setRefreshableConfigField(t, authCfg, "github.com", "test-user", "refresh_token", "refresh")

	require.NoError(t, authCfg.activateUser("github.com", "test-user"))

	activeUser, err := authCfg.ActiveUser("github.com")
	require.NoError(t, err)
	require.Equal(t, "test-user", activeUser)

	cred := authCfg.ActiveToken("github.com")
	token, source := cred.Token, cred.Source
	require.Equal(t, "access", token)
	require.Equal(t, "refreshable_oauth_token", source)
}

func TestLogoutRemovesAllUserCredentialRecords(t *testing.T) {
	// This populates every credential representation the write paths can produce for the user at once: a non-expiring
	// token and a refreshable credential, in both the keyring and the config file. The non-expiring token also has a
	// host active ("") slot, which refreshable credentials do not use. That combined state is not realistic - the
	// write paths keep the storage forms mutually exclusive - but populating it lets the test prove that Logout leaves
	// nothing behind for the user on the host.
	authCfg := newTestAuthConfig(t)

	credential := gh.Credential{Token: "access", RefreshToken: "refresh"}

	// Non-expiring tokens in the keyring (per-user and host active slot).
	require.NoError(t, keyring.Set(keyringServiceName("github.com"), "test-user", "keyring-user-token"))
	require.NoError(t, keyring.Set(keyringServiceName("github.com"), "", "keyring-active-token"))

	// Refreshable credential in the keyring (per-user slot only).
	setRefreshableKeyring(t, "github.com", "test-user", credential)

	// Non-expiring tokens in the config (per-user and host active slot).
	authCfg.cfg.Set([]string{hostsKey, "github.com", usersKey, "test-user", oauthTokenKey}, "config-user-token")
	authCfg.cfg.Set([]string{hostsKey, "github.com", oauthTokenKey}, "config-active-token")

	// Refreshable credential in the config (per-user slot only).
	setRefreshableConfigField(t, authCfg, "github.com", "test-user", "access_token", "access")
	setRefreshableConfigField(t, authCfg, "github.com", "test-user", "refresh_token", "refresh")

	authCfg.cfg.Set([]string{hostsKey, "github.com", userKey}, "test-user")

	require.NoError(t, authCfg.Logout("github.com", "test-user"))

	// Every non-expiring keyring record for the host, per-user and host active, is gone.
	for _, user := range []string{"test-user", ""} {
		_, err := keyring.Get(keyringServiceName("github.com"), user)
		require.Error(t, err)
	}
	// The per-user refreshable keyring record is gone.
	_, err := keyring.Get(refreshableServiceName("github.com"), "test-user")
	require.Error(t, err)

	// The whole host entry, and therefore every config-stored credential, is gone.
	_, err = authCfg.cfg.Get([]string{hostsKey, "github.com"})
	require.Error(t, err)
	require.Empty(t, authCfg.UsersForHost("github.com"))
}

func TestLogoutAcquiresRefreshLock(t *testing.T) {
	// Logout must run under the same lock the refresh path uses so it wins any race with an in-flight refresh. Pin
	// that it acquires and releases the shared refresh lock; without the fix Logout mutates the config without ever
	// touching the lock and this fails.
	authCfg := newTestAuthConfig(t)
	_, err := authCfg.Login("github.com", "test-user", "gho_token", "https", false)
	require.NoError(t, err)

	var locked, unlocked bool
	authCfg.acquireRefreshLock = func(ctx context.Context, path string) (func(), error) {
		locked = true
		return func() { unlocked = true }, nil
	}

	require.NoError(t, authCfg.Logout("github.com", "test-user"))
	require.True(t, locked, "logout must acquire the shared refresh lock")
	require.True(t, unlocked, "logout must release the shared refresh lock")
}

func TestLogoutReloadsFreshConfigBeforeRemovingCredentials(t *testing.T) {
	// A concurrent refresh can persist a rotated credential while logout is in flight. Because logout reloads the
	// freshest config under the lock before deleting, it sees and removes whatever the refresh just wrote. Pin that
	// reload happens, and that it happens before the deletion, so logout never leaves a freshly persisted credential
	// behind. Without the fix Logout never reloads and this fails.
	authCfg := newTestAuthConfig(t)
	setRefreshableConfigField(t, authCfg, "github.com", "test-user", "access_token", "access")
	setRefreshableConfigField(t, authCfg, "github.com", "test-user", "refresh_token", "refresh")
	authCfg.cfg.Set([]string{hostsKey, "github.com", userKey}, "test-user")

	reloaded := false
	authCfg.reloadConfig = func() error {
		reloaded = true
		// Reload runs under the lock before any deletion, so the credential is still present at this point.
		_, err := authCfg.cfg.Get(append(refreshablePath("github.com", "test-user"), "access_token"))
		require.NoError(t, err, "reload must run before logout removes the credential")
		return nil
	}

	require.NoError(t, authCfg.Logout("github.com", "test-user"))
	require.True(t, reloaded, "logout must reload the config under the refresh lock")

	_, err := authCfg.cfg.Get([]string{hostsKey, "github.com"})
	require.Error(t, err)
	require.Empty(t, authCfg.UsersForHost("github.com"))
}

func TestLoginAcquiresRefreshLock(t *testing.T) {
	// Login must run under the same lock the refresh path uses so it serializes against an in-flight refresh and wins
	// either ordering. Pin that it acquires and releases the shared refresh lock; without the fix Login mutates the
	// config without ever touching the lock and this fails.
	authCfg := newTestAuthConfig(t)

	var locked, unlocked bool
	authCfg.acquireRefreshLock = func(ctx context.Context, path string) (func(), error) {
		locked = true
		return func() { unlocked = true }, nil
	}

	_, err := authCfg.Login("github.com", "test-user", "gho_token", "https", false)
	require.NoError(t, err)
	require.True(t, locked, "login must acquire the shared refresh lock")
	require.True(t, unlocked, "login must release the shared refresh lock")
}

func TestLoginProceedsWhenRefreshLockCannotBeAcquired(t *testing.T) {
	// The cross-process lock is best effort: a filesystem without advisory locking (or a peer holding the lock past
	// the timeout) must not block an otherwise valid login. When acquisition fails the helper reports lockHeld=false
	// and still runs the body, so login proceeds and stores the credential. Without that, login would be unusable
	// wherever flock is unavailable.
	authCfg := newTestAuthConfig(t)

	var released bool
	authCfg.acquireRefreshLock = func(ctx context.Context, path string) (func(), error) {
		return func() { released = true }, errors.New("advisory locking unavailable")
	}

	insecureStorageUsed, err := authCfg.Login("github.com", "test-user", "gho_token", "https", false)
	require.NoError(t, err)
	require.True(t, insecureStorageUsed)
	require.False(t, released, "no release func should run when the lock was never acquired")

	token, err := authCfg.cfg.Get([]string{hostsKey, "github.com", usersKey, "test-user", oauthTokenKey})
	require.NoError(t, err)
	require.Equal(t, "gho_token", token)
}

func TestLoginReloadsFreshConfigBeforeMutating(t *testing.T) {
	// A concurrent refresh can persist a rotated credential while login is in flight. Because login reloads the
	// freshest config under the lock before it stores the new credential and purges the refreshable record, it sees
	// and supersedes whatever the refresh just wrote. Pin that reload happens, and that it happens before the
	// mutation, so login wins the race. Without the wrap Login never reloads and this fails.
	authCfg := newTestAuthConfig(t)

	reloaded := false
	authCfg.reloadConfig = func() error {
		reloaded = true
		// Reload runs under the lock before any mutation, so the user's token slot is still empty at this point.
		_, err := authCfg.cfg.Get([]string{hostsKey, "github.com", usersKey, "test-user", oauthTokenKey})
		require.Error(t, err, "reload must run before login stores the credential")
		return nil
	}

	_, err := authCfg.Login("github.com", "test-user", "gho_token", "https", false)
	require.NoError(t, err)
	require.True(t, reloaded, "login must reload the config under the refresh lock")

	token, err := authCfg.cfg.Get([]string{hostsKey, "github.com", usersKey, "test-user", oauthTokenKey})
	require.NoError(t, err)
	require.Equal(t, "gho_token", token)
}

func TestLoginSucceedsWhenKeyringUnavailable(t *testing.T) {
	// The refreshable-record purge is best effort against capability loss: an unavailable keyring must not fail an
	// otherwise valid insecure login. refreshableFromKeyring collapses an unavailable keyring to "not found", so the
	// purge's post-condition check resolves no surviving record and login stays silent. Without that, surfacing the
	// keyring delete error would break login wherever the keyring is absent.
	authCfg := newTestAuthConfig(t)
	keyring.MockInitWithError(errors.New("keyring is unavailable"))

	insecureStorageUsed, err := authCfg.Login("github.com", "test-user", "gho_token", "https", false)
	require.NoError(t, err)
	require.True(t, insecureStorageUsed)

	token, err := authCfg.cfg.Get([]string{hostsKey, "github.com", usersKey, "test-user", oauthTokenKey})
	require.NoError(t, err)
	require.Equal(t, "gho_token", token)
}

func TestSwitchUserActivatesRefreshableCredential(t *testing.T) {
	authCfg := newTestAuthConfig(t)
	_, err := authCfg.Login("github.com", "test-user-2", "test-token-2", "ssh", false)
	require.NoError(t, err)

	authCfg.cfg.Set([]string{hostsKey, "github.com", usersKey, "test-user-1"}, "")
	setRefreshableConfigField(t, authCfg, "github.com", "test-user-1", "access_token", "access")
	setRefreshableConfigField(t, authCfg, "github.com", "test-user-1", "refresh_token", "refresh")

	require.NoError(t, authCfg.SwitchUser("github.com", "test-user-1"))

	cred := authCfg.ActiveToken("github.com")
	token, source := cred.Token, cred.Source
	require.Equal(t, "access", token)
	require.Equal(t, "refreshable_oauth_token", source)

	activeUser, err := authCfg.ActiveUser("github.com")
	require.NoError(t, err)
	require.Equal(t, "test-user-1", activeUser)
}

func TestLoginRefreshable(t *testing.T) {
	expiry := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	tests := []struct {
		name       string
		secure     bool
		keyringErr bool
		wantSecure bool
	}{
		{name: "secure storage", secure: true, wantSecure: true},
		{name: "insecure storage", secure: false, wantSecure: false},
		{name: "secure storage falls back when keyring errors", secure: true, keyringErr: true, wantSecure: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authCfg := newTestAuthConfig(t)
			if tt.keyringErr {
				keyring.MockInitWithError(errors.New("keyring is unavailable"))
			}
			credential := gh.Credential{
				Token:                 "access-token",
				RefreshToken:          "refresh-token",
				ExpiresIn:             3600,
				ExpiresAt:             &expiry,
				RefreshTokenExpiresIn: 86400,
				RefreshTokenExpiresAt: &expiry,
			}

			insecureStorageUsed, err := authCfg.LoginRefreshable("github.com", "test-user", credential, "ssh", tt.secure)
			require.NoError(t, err)
			require.Equal(t, tt.wantSecure, !insecureStorageUsed)

			gotCredential, secure, err := authCfg.refreshableForUser("github.com", "test-user")
			require.NoError(t, err)
			require.Equal(t, tt.wantSecure, secure)

			// refreshableForUser tags the credential with the source it was read from, so compare against the stored
			// credential with the expected source filled in.
			wantCredential := credential
			if tt.wantSecure {
				wantCredential.Source = "keyring"
			} else {
				wantCredential.Source = "refreshable_oauth_token"
			}
			require.Equal(t, wantCredential, gotCredential)

			// The user is active and the credential resolves as the active token.
			activeUser, err := authCfg.ActiveUser("github.com")
			require.NoError(t, err)
			require.Equal(t, "test-user", activeUser)

			token := authCfg.ActiveToken("github.com").Token
			require.Equal(t, "access-token", token)

			requireKeyWithValue(t, authCfg.cfg, []string{hostsKey, "github.com", gitProtocolKey}, "ssh")
		})
	}
}

func TestLoginRefreshableRejectsInvalidCredential(t *testing.T) {
	valid := gh.Credential{Token: "access", RefreshToken: "refresh"}
	tests := []struct {
		name       string
		hostname   string
		username   string
		credential gh.Credential
	}{
		{name: "empty hostname", hostname: "", username: "test-user", credential: valid},
		{name: "empty username", hostname: "github.com", username: "", credential: valid},
		{name: "missing access token", hostname: "github.com", username: "test-user", credential: gh.Credential{RefreshToken: "refresh"}},
		{name: "missing refresh token", hostname: "github.com", username: "test-user", credential: gh.Credential{Token: "access"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authCfg := newTestAuthConfig(t)

			_, err := authCfg.LoginRefreshable(tt.hostname, tt.username, tt.credential, "ssh", true)

			require.Error(t, err)
		})
	}
}

func TestLoginRefreshablePurgesNonExpiringLogin(t *testing.T) {
	authCfg := newTestAuthConfig(t)

	// A prior non-expiring login persists a token to disk so the reload observes it.
	_, err := authCfg.Login("github.com", "test-user", "stale-token", "ssh", false)
	require.NoError(t, err)

	credential := gh.Credential{Token: "access", RefreshToken: "refresh"}
	_, err = authCfg.LoginRefreshable("github.com", "test-user", credential, "ssh", false)
	require.NoError(t, err)

	// The stale non-expiring token is gone and the refreshable credential resolves instead.
	requireNoKey(t, authCfg.cfg, []string{hostsKey, "github.com", usersKey, "test-user", oauthTokenKey})
	cred, err := authCfg.TokenForUser("github.com", "test-user")
	require.NoError(t, err)
	require.Equal(t, "access", cred.Token)
}

// TestLoginRefreshablePurgesNonExpiringKeyringLogin covers the keyring arm of the non-expiring purge. Resolution now
// probes the ordinary keyring before the refreshable credential, so a stale per-user keyring token left behind would
// outrank the refreshable one; the purge must clear it for the refreshable credential to resolve.
func TestLoginRefreshablePurgesNonExpiringKeyringLogin(t *testing.T) {
	authCfg := newTestAuthConfig(t)

	// A prior non-expiring login stores a token in the keyring per-user slot.
	_, err := authCfg.Login("github.com", "test-user", "stale-token", "ssh", true)
	require.NoError(t, err)

	credential := gh.Credential{Token: "access", RefreshToken: "refresh"}
	_, err = authCfg.LoginRefreshable("github.com", "test-user", credential, "ssh", true)
	require.NoError(t, err)

	// The stale keyring token is gone and the refreshable credential resolves instead.
	_, err = keyring.Get(keyringServiceName("github.com"), "test-user")
	require.Error(t, err)
	cred, err := authCfg.TokenForUser("github.com", "test-user")
	require.NoError(t, err)
	require.Equal(t, "access", cred.Token)
}

// TestLoginPurgesRefreshableRecord is the inverse of TestLoginRefreshablePurgesNonExpiringLogin: logging in with a
// non-refreshable token must remove the user's refreshable record so the two kinds never coexist. The regression it
// guards (cli/cli#14451 review): a leftover refreshable record short-circuits activateUser, so the new token is never
// copied into the host-level oauth_token slot and go-gh extensions can no longer read a token.
func TestLoginPurgesRefreshableRecord(t *testing.T) {
	authCfg := newTestAuthConfig(t)

	// A prior expiring login stores a refreshable credential in the keyring.
	credential := gh.Credential{Token: "access", RefreshToken: "refresh"}
	_, err := authCfg.LoginRefreshable("github.com", "test-user", credential, "ssh", true)
	require.NoError(t, err)

	// Switching back to a non-refreshable token with insecure storage must purge the refreshable record.
	_, err = authCfg.Login("github.com", "test-user", "plain-token", "ssh", false)
	require.NoError(t, err)

	// The refreshable record is gone from both storage forms.
	_, err = refreshableFromKeyring("github.com", "test-user")
	require.ErrorIs(t, err, errRefreshableNotFound)
	requireNoKey(t, authCfg.cfg, refreshablePath("github.com", "test-user"))

	// TokenForUser resolves the new non-refreshable token.
	cred, err := authCfg.TokenForUser("github.com", "test-user")
	require.NoError(t, err)
	require.Equal(t, "plain-token", cred.Token)

	// And it was copied into the host-level oauth_token slot that go-gh reads for extensions, which regressed before
	// this fix because the stale refreshable record short-circuited activation.
	hostToken, err := authCfg.cfg.Get([]string{hostsKey, "github.com", oauthTokenKey})
	require.NoError(t, err)
	require.Equal(t, "plain-token", hostToken)
}

// TestLoginFailsWhenOtherCredentialKindSurvivesPurge covers the purge post-condition checks: an OS keyring can report a
// delete as successful while the secret remains, and the surviving credential would then outrank the new login. Login
// must surface that instead of activating the user and reporting success.
func TestLoginFailsWhenOtherCredentialKindSurvivesPurge(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T)
		login   func(authCfg *AuthConfig) error
		wantErr string
	}{
		{
			name: "non-expiring keyring token survives a refreshable login",
			setup: func(t *testing.T) {
				require.NoError(t, keyring.Set(keyringServiceName("github.com"), "test-user", "stale-token"))
			},
			login: func(authCfg *AuthConfig) error {
				credential := gh.Credential{Token: "access", RefreshToken: "refresh"}
				_, err := authCfg.LoginRefreshable("github.com", "test-user", credential, "ssh", true)
				return err
			},
			wantErr: "could not remove the existing non-expiring credential, which would shadow this login",
		},
		{
			name: "refreshable keyring record survives a non-expiring login",
			setup: func(t *testing.T) {
				setRefreshableKeyring(t, "github.com", "test-user", gh.Credential{Token: "access", RefreshToken: "refresh"})
			},
			login: func(authCfg *AuthConfig) error {
				_, err := authCfg.Login("github.com", "test-user", "plain-token", "ssh", false)
				return err
			},
			wantErr: "could not remove the existing refreshable credential, which would shadow this login",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authCfg := newTestAuthConfig(t)
			tt.setup(t)

			originalKeyringDelete := keyringDelete
			t.Cleanup(func() { keyringDelete = originalKeyringDelete })
			keyringDelete = func(service, user string) error { return nil }

			err := tt.login(authCfg)
			require.EqualError(t, err, tt.wantErr)

			// The user was never activated.
			_, err = authCfg.ActiveUser("github.com")
			require.Error(t, err)
		})
	}
}

func TestWithFreshConfigUnderLockInvokesReloadBeforeFn(t *testing.T) {
	authCfg := newTestAuthConfig(t)

	var order []string
	authCfg.reloadConfig = func() error {
		order = append(order, "reload")
		return nil
	}

	err := authCfg.withFreshConfigForRefresh(func(lockHeld bool) error {
		order = append(order, "fn")
		return nil
	})
	require.NoError(t, err)

	require.Equal(t, []string{"reload", "fn"}, order)
}

func TestWithFreshConfigUnderLockPropagatesReloadError(t *testing.T) {
	authCfg := newTestAuthConfig(t)
	sentinel := errors.New("reload boom")
	authCfg.reloadConfig = func() error { return sentinel }

	fnRan := false
	err := authCfg.withFreshConfigForRefresh(func(lockHeld bool) error {
		fnRan = true
		return nil
	})

	require.ErrorIs(t, err, sentinel)
	require.False(t, fnRan, "fn must not run when the reload fails")
}

func TestWithFreshConfigUnderLockPropagatesError(t *testing.T) {
	authCfg := newTestAuthConfig(t)
	sentinel := errors.New("boom")

	err := authCfg.withFreshConfigForRefresh(func(lockHeld bool) error { return sentinel })

	require.ErrorIs(t, err, sentinel)
}

func TestFlockRefreshLockCreatesMissingDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "gh", refreshLockFile)

	unlock, err := flockRefreshLock(t.Context(), path)
	require.NoError(t, err)
	defer unlock()

	require.FileExists(t, path)
}

func TestWithFreshConfigUnderLockProceedsWhenStateDirCannotBeCreated(t *testing.T) {
	// If we have reached a refresh, the environment is already set up enough to proceed. A read-only mount (or any
	// other reason the state directory cannot be created) must not fail the user's command; we degrade to running
	// without the lock, exactly as when the lock cannot be acquired.
	authCfg := newTestAuthConfig(t)
	// Use the real acquirer, since creating the state directory happens behind the lock seam.
	authCfg.acquireRefreshLock = flockRefreshLock

	// Point the state dir under a regular file so MkdirAll fails with ENOTDIR regardless of privileges (running as
	// root would bypass a read-only directory bit, but never a non-directory path component).
	file := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(file, nil, 0600))
	// go-gh checks XDG_STATE_HOME first on every platform (before the Windows-only LocalAppData), so this overrides
	// the state dir regardless of the OS the test runs on.
	t.Setenv("XDG_STATE_HOME", filepath.Join(file, "state"))

	ran := false
	observedLockHeld := true
	err := authCfg.withFreshConfigForRefresh(func(lockHeld bool) error {
		ran = true
		observedLockHeld = lockHeld
		return nil
	})

	require.NoError(t, err)
	require.True(t, ran, "fn must still run when the state directory cannot be created")
	require.False(t, observedLockHeld, "the lock cannot be held when its directory cannot be created")
}

func TestWithFreshConfigUnderLockSerializesInProcess(t *testing.T) {
	authCfg := newTestAuthConfig(t)

	const goroutines = 50
	var (
		wg        sync.WaitGroup
		active    atomic.Int32
		maxActive atomic.Int32
		completed int
	)

	for range goroutines {
		wg.Go(func() {
			_ = authCfg.withFreshConfigForRefresh(func(lockHeld bool) error {
				n := active.Add(1)
				for {
					m := maxActive.Load()
					if n <= m || maxActive.CompareAndSwap(m, n) {
						break
					}
				}
				// completed is intentionally a plain int: it is only safe to touch because the mutex under test
				// serializes fn, so a data race here would signal broken mutual exclusion.
				completed++
				time.Sleep(time.Millisecond)
				active.Add(-1)
				return nil
			})
		})
	}
	wg.Wait()

	require.Equal(t, int32(1), maxActive.Load(), "fn ran concurrently, in-process serialization is broken")
	require.Equal(t, goroutines, completed)
}

// renewingRefresher returns a mock refresher that always renews to a fresh, long lived credential.
func renewingRefresher() *ghmock.TokenRefresherMock {
	future := time.Now().Add(time.Hour)
	return &ghmock.TokenRefresherMock{
		RefreshFunc: func(refreshToken string, hostname string) (gh.RefreshableCredential, error) {
			return gh.RefreshableCredential{
				AccessToken:  "new-access",
				RefreshToken: "new-refresh",
				ExpiresAt:    &future,
			}, nil
		},
	}
}

func loginRefreshableWithExpiry(t *testing.T, authCfg *AuthConfig, hostname, username, access, refresh string, expiresAt time.Time, secure bool) {
	t.Helper()
	_, err := authCfg.LoginRefreshable(hostname, username, gh.Credential{
		Token:        access,
		RefreshToken: refresh,
		ExpiresAt:    &expiresAt,
	}, "https", secure)
	require.NoError(t, err)
}

func TestActiveTokenWithRefresh(t *testing.T) {
	host := "github.com"
	user := "test-user"

	t.Run("refreshes and persists an expired refreshable credential", func(t *testing.T) {
		authCfg := newTestAuthConfig(t)
		refresher := renewingRefresher()
		authCfg.SetTokenRefresher(refresher)
		loginRefreshableWithExpiry(t, authCfg, host, user, "old-access", "old-refresh", time.Now().Add(-time.Hour), false)

		credential, status, err := authCfg.ActiveTokenWithRefresh(host)

		require.NoError(t, err)
		require.Equal(t, gh.RefreshStatusDone, status)
		require.Equal(t, "new-access", credential.Token)
		require.Equal(t, "new-refresh", credential.RefreshToken)
		// The renewed credential is persisted, so a subsequent read returns it rather than the stale one.
		require.Equal(t, "new-access", authCfg.ActiveToken(host).Token)
		require.Len(t, refresher.RefreshCalls(), 1)
		require.Equal(t, "old-refresh", refresher.RefreshCalls()[0].RefreshToken)
		require.Equal(t, host, refresher.RefreshCalls()[0].Hostname)
	})

	t.Run("refreshes and persists to secure storage", func(t *testing.T) {
		authCfg := newTestAuthConfig(t)
		refresher := renewingRefresher()
		authCfg.SetTokenRefresher(refresher)
		loginRefreshableWithExpiry(t, authCfg, host, user, "old-access", "old-refresh", time.Now().Add(-time.Hour), true)

		credential, status, err := authCfg.ActiveTokenWithRefresh(host)

		require.NoError(t, err)
		require.Equal(t, gh.RefreshStatusDone, status)
		require.Equal(t, "new-access", credential.Token)
		// The renewed credential is written back to the keyring it was read from.
		stored, secure, err := authCfg.refreshableForUser(host, user)
		require.NoError(t, err)
		require.True(t, secure)
		require.Equal(t, "new-access", stored.Token)
		require.Len(t, refresher.RefreshCalls(), 1)
	})

	t.Run("does not refresh a credential that is still valid", func(t *testing.T) {
		authCfg := newTestAuthConfig(t)
		refresher := renewingRefresher()
		authCfg.SetTokenRefresher(refresher)
		loginRefreshableWithExpiry(t, authCfg, host, user, "old-access", "old-refresh", time.Now().Add(time.Hour), false)

		credential, status, err := authCfg.ActiveTokenWithRefresh(host)

		require.NoError(t, err)
		require.Equal(t, gh.RefreshStatusUnnecessary, status)
		require.Equal(t, "old-access", credential.Token)
		require.Empty(t, refresher.RefreshCalls())
	})

	t.Run("reports failure and keeps the old credential when the exchange fails", func(t *testing.T) {
		authCfg := newTestAuthConfig(t)
		authCfg.SetTokenRefresher(&ghmock.TokenRefresherMock{
			RefreshFunc: func(refreshToken string, hostname string) (gh.RefreshableCredential, error) {
				return gh.RefreshableCredential{}, errors.New("token endpoint unreachable")
			},
		})
		loginRefreshableWithExpiry(t, authCfg, host, user, "old-access", "old-refresh", time.Now().Add(-time.Hour), false)

		credential, status, err := authCfg.ActiveTokenWithRefresh(host)

		require.Error(t, err)
		require.Equal(t, gh.RefreshStatusFailed, status)
		require.Equal(t, "old-access", credential.Token)
		// The stale credential is left untouched so a later attempt can retry.
		require.Equal(t, "old-access", authCfg.ActiveToken(host).Token)
	})

	t.Run("reports expiry and removes the credential when the refresh token is rejected", func(t *testing.T) {
		authCfg := newTestAuthConfig(t)
		refresher := &ghmock.TokenRefresherMock{
			RefreshFunc: func(refreshToken string, hostname string) (gh.RefreshableCredential, error) {
				return gh.RefreshableCredential{}, fmt.Errorf("server rejected token: %w", gh.ErrRefreshTokenInvalid)
			},
		}
		authCfg.SetTokenRefresher(refresher)
		loginRefreshableWithExpiry(t, authCfg, host, user, "old-access", "old-refresh", time.Now().Add(-time.Hour), false)

		credential, status, err := authCfg.ActiveTokenWithRefresh(host)

		require.ErrorIs(t, err, gh.ErrRefreshTokenInvalid)
		require.Equal(t, gh.RefreshStatusExpired, status)
		require.Empty(t, credential.Token)
		// The rejected credential is wiped, so it can no longer be resolved.
		require.Empty(t, authCfg.ActiveToken(host).Token)
		require.False(t, authCfg.HasActiveToken(host))

		// A subsequent refresh attempt finds no credential and does not call the refresher again, so a dead refresh
		// token cannot repeatedly hit the token endpoint.
		credential, status, err = authCfg.ActiveTokenWithRefresh(host)
		require.NoError(t, err)
		require.Equal(t, gh.RefreshStatusInapplicable, status)
		require.Empty(t, credential.Token)
		require.Len(t, refresher.RefreshCalls(), 1)
	})

	t.Run("reports expiry but leaves the credential when the refresh token is rejected without the lock", func(t *testing.T) {
		// Without the cross-process lock a rejection is ambiguous: a peer may have already spent this single-use
		// refresh token and be about to persist the replacement. Destroying storage here could race that peer and
		// discard a valid credential, so the rejected process must leave storage untouched while still reporting the
		// rejection. Contrast with the lock-held case above, which does remove the credential.
		authCfg := newTestAuthConfig(t)
		authCfg.acquireRefreshLock = func(ctx context.Context, path string) (func(), error) {
			return nil, errors.New("advisory locking unavailable")
		}
		refresher := &ghmock.TokenRefresherMock{
			RefreshFunc: func(refreshToken string, hostname string) (gh.RefreshableCredential, error) {
				return gh.RefreshableCredential{}, fmt.Errorf("server rejected token: %w", gh.ErrRefreshTokenInvalid)
			},
		}
		authCfg.SetTokenRefresher(refresher)
		loginRefreshableWithExpiry(t, authCfg, host, user, "old-access", "old-refresh", time.Now().Add(-time.Hour), false)

		credential, status, err := authCfg.ActiveTokenWithRefresh(host)

		require.ErrorIs(t, err, gh.ErrRefreshTokenInvalid)
		require.Equal(t, gh.RefreshStatusExpired, status)
		require.Empty(t, credential.Token)

		// The stored credential is untouched, so it still resolves and a peer's replacement write (or a retry once
		// the lock is available) can still land.
		stored, _, err := authCfg.refreshableForUser(host, user)
		require.NoError(t, err)
		require.Equal(t, "old-refresh", stored.RefreshToken)

		// Because the credential was not removed, a subsequent attempt still reaches the refresher rather than
		// short-circuiting on a missing credential.
		_, _, _ = authCfg.ActiveTokenWithRefresh(host)
		require.Len(t, refresher.RefreshCalls(), 2)
	})

	t.Run("cannot refresh when no refresher is configured", func(t *testing.T) {
		authCfg := newTestAuthConfig(t)
		loginRefreshableWithExpiry(t, authCfg, host, user, "old-access", "old-refresh", time.Now().Add(-time.Hour), false)

		credential, status, err := authCfg.ActiveTokenWithRefresh(host)

		require.NoError(t, err)
		require.Equal(t, gh.RefreshStatusInapplicable, status)
		require.Equal(t, "old-access", credential.Token)
	})

	t.Run("is inapplicable for a non-refreshable credential", func(t *testing.T) {
		authCfg := newTestAuthConfig(t)
		refresher := renewingRefresher()
		authCfg.SetTokenRefresher(refresher)
		_, err := authCfg.Login(host, user, "plain-token", "https", false)
		require.NoError(t, err)

		credential, status, err := authCfg.ActiveTokenWithRefresh(host)

		require.NoError(t, err)
		require.Equal(t, gh.RefreshStatusInapplicable, status)
		require.Equal(t, "plain-token", credential.Token)
		require.Empty(t, refresher.RefreshCalls())
	})
}

func TestTokenForUserWithRefresh(t *testing.T) {
	host := "github.com"
	user := "test-user"

	t.Run("refreshes and persists an expired refreshable credential", func(t *testing.T) {
		authCfg := newTestAuthConfig(t)
		refresher := renewingRefresher()
		authCfg.SetTokenRefresher(refresher)
		loginRefreshableWithExpiry(t, authCfg, host, user, "old-access", "old-refresh", time.Now().Add(-time.Hour), false)

		credential, status, err := authCfg.TokenForUserWithRefresh(host, user)

		require.NoError(t, err)
		require.Equal(t, gh.RefreshStatusDone, status)
		require.Equal(t, "new-access", credential.Token)
		cred, err := authCfg.TokenForUser(host, user)
		require.NoError(t, err)
		require.Equal(t, "new-access", cred.Token)
	})

	t.Run("does not refresh a credential that is still valid", func(t *testing.T) {
		authCfg := newTestAuthConfig(t)
		refresher := renewingRefresher()
		authCfg.SetTokenRefresher(refresher)
		loginRefreshableWithExpiry(t, authCfg, host, user, "old-access", "old-refresh", time.Now().Add(time.Hour), false)

		credential, status, err := authCfg.TokenForUserWithRefresh(host, user)

		require.NoError(t, err)
		require.Equal(t, gh.RefreshStatusUnnecessary, status)
		require.Equal(t, "old-access", credential.Token)
		require.Empty(t, refresher.RefreshCalls())
	})

	t.Run("is inapplicable for a non-refreshable credential", func(t *testing.T) {
		authCfg := newTestAuthConfig(t)
		_, err := authCfg.Login(host, user, "plain-token", "https", false)
		require.NoError(t, err)

		credential, status, err := authCfg.TokenForUserWithRefresh(host, user)

		require.NoError(t, err)
		require.Equal(t, gh.RefreshStatusInapplicable, status)
		require.Equal(t, "plain-token", credential.Token)
	})

	t.Run("propagates the read error when the user has no token", func(t *testing.T) {
		authCfg := newTestAuthConfig(t)

		credential, status, err := authCfg.TokenForUserWithRefresh(host, "missing-user")

		require.Error(t, err)
		require.Equal(t, gh.RefreshStatusInapplicable, status)
		require.Empty(t, credential.Token)
	})
}
