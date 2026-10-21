package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/cli/cli/v2/internal/flock"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/keyring"
	ghConfig "github.com/cli/go-gh/v2/pkg/config"
)

// refreshableTokenKey is the config and keyring field that holds a refreshable
// credential record, kept separate from the non-expiring oauth_token so that neither
// old readers nor go-gh mistake the record for a bare token string.
const refreshableTokenKey = "refreshable_oauth_token"

// errRefreshableNotFound reports that no refreshable credential exists for the
// requested slot. It is distinct from other errors so callers can fall back to
// non-expiring credentials without masking real read failures.
var errRefreshableNotFound = errors.New("no refreshable credentials found")

// timeNow returns the current time. It is a package-level indirection so tests can pin the clock when exercising
// expiry-based refresh decisions.
var timeNow = time.Now

// refreshLockFile names the cross-process lock, in the config directory, that serializes token refreshes. Only the
// refresh path takes it because refreshing is the sole operation that spends a single-use rotating refresh token;
// other config mutations such as auth login or logout overwrite credentials wholesale and are safe under
// last-writer-wins, so a single refresh-scoped lock is enough.
const refreshLockFile = "refresh.lock"

// refreshLockTimeout bounds how long withFreshConfigForRefresh waits for the cross-process lock before giving up and
// proceeding without it. It is long enough to outlast a normal concurrent refresh yet short enough that a stuck or
// unsupported lock cannot wedge gh indefinitely.
const refreshLockTimeout = 60 * time.Second

// flockRefreshLock acquires the cross-process refresh lock with flock. It is the production implementation used when
// AuthConfig.acquireRefreshLock is nil.
func flockRefreshLock(ctx context.Context, path string) (unlock func(), err error) {
	_, unlock, err = flock.Lock(ctx, path)
	return unlock, err
}

// withFreshConfigForRefresh runs fn, a token-refresh read-modify-write, while holding both an in-process mutex and a
// cross-process advisory lock, and reloads the configuration from disk before running fn so it observes any credential
// another process persisted. The mutex serializes goroutines sharing this AuthConfig; the file lock serializes
// separate gh processes. Together they prevent concurrent refreshes from each spending a rotating refresh token based
// on stale state. fn runs with both locks held and should perform the credential read, token exchange, and write as
// one unit.
//
// The file lock is best effort: if it cannot be acquired (for example on a filesystem without advisory locking, or if
// another process holds it past refreshLockTimeout), fn still runs so a filesystem limitation never blocks the user.
//
// The reload refreshes the shared configuration in place, so any config changes not yet written to disk are discarded.
// That is acceptable on the refresh path because mutations are persisted as they happen.
func (c *AuthConfig) withFreshConfigForRefresh(fn func() error) error {
	c.refreshMu.Lock()
	defer c.refreshMu.Unlock()

	dir := ghConfig.ConfigDir()
	if err := os.MkdirAll(dir, 0771); err != nil {
		return fmt.Errorf("could not create config directory for the refresh lock: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), refreshLockTimeout)
	defer cancel()

	acquire := c.acquireRefreshLock
	if acquire == nil {
		acquire = flockRefreshLock
	}

	unlock, err := acquire(ctx, filepath.Join(dir, refreshLockFile))
	if err != nil {
		// We could not acquire the cross-process lock, either because the filesystem does not support advisory
		// locking (some network or FUSE mounts) or because another process held it past refreshLockTimeout. We
		// deliberately ignore the error and proceed rather than fail the user's command over a filesystem
		// limitation. The residual risk is two processes refreshing at once and spending a rotating token twice.
		unlock = func() {}
	}
	defer unlock()

	if err := c.reload(); err != nil {
		return fmt.Errorf("could not reload configuration: %w", err)
	}

	return fn()
}

// reload re-reads the config from disk into the shared config, using the reloadConfig seam when set and the real
// ghConfig.Reload otherwise. The production fallback passes fallbackConfig so a config file deleted between reads is
// restored to gh's defaults, matching NewConfig.
func (c *AuthConfig) reload() error {
	if c.reloadConfig != nil {
		return c.reloadConfig()
	}
	return ghConfig.Reload(fallbackConfig())
}

// LoginRefreshable stores a refreshable credential for the user, sets the git protocol, and activates the user. It
// mirrors Login: secure storage is preferred and the insecure configuration is used as a fallback, reported via the
// returned bool. Any non-expiring credential for the user is removed so it cannot outrank the refreshable record. It
// runs under the cross-process refresh lock and reloads the configuration first, so it cannot interleave with a
// concurrent configuration read-modify-write in another gh process.
func (c *AuthConfig) LoginRefreshable(hostname, username string, credential gh.Credential, gitProtocol string, secureStorage bool) (bool, error) {
	var insecureStorageUsed bool
	err := c.withFreshConfigForRefresh(func() error {
		var err error
		insecureStorageUsed, err = c.loginRefreshable(hostname, username, credential, gitProtocol, secureStorage)
		return err
	})
	return insecureStorageUsed, err
}

func (c *AuthConfig) loginRefreshable(hostname, username string, credential gh.Credential, gitProtocol string, secureStorage bool) (bool, error) {
	if hostname == "" || username == "" {
		return false, errors.New("hostname and username are required")
	}
	if err := validateRefreshableCredential(credential); err != nil {
		return false, err
	}

	insecureStorageUsed := false
	var setErr error
	if secureStorage {
		setErr = c.setRefreshableInKeyring(hostname, username, credential)
		if setErr == nil {
			_ = c.cfg.Remove(refreshablePath(hostname, username))
		}
	}
	if !secureStorage || setErr != nil {
		c.setRefreshableInConfig(hostname, username, credential)
		_ = keyring.Delete(refreshableServiceName(hostname), username)
		insecureStorageUsed = true
	}

	// Remove any non-expiring credential for this user so a stale token cannot be resolved in preference to the
	// refreshable record.
	_ = c.cfg.Remove([]string{hostsKey, hostname, usersKey, username, oauthTokenKey})
	_ = keyring.Delete(keyringServiceName(hostname), username)

	return insecureStorageUsed, c.completeLogin(hostname, username, gitProtocol)
}

// refreshableForUser returns the refreshable credential stored for the user along with whether it came from secure
// storage. It prefers the secure keyring and falls back to the insecure configuration record, returning
// errRefreshableNotFound when neither exists.
func (c *AuthConfig) refreshableForUser(hostname, username string) (gh.Credential, bool, error) {
	// Consult the secure keyring first, matching TokenForUser. The two storage forms are mutually exclusive, so this
	// never changes which credential is returned in practice, but keeping the precedence identical avoids confusing
	// bugs if a misconfiguration ever left records in both places. Preferring the keyring also means the refresh
	// tokens survive a corrupted or tampered config file.
	credential, err := refreshableFromKeyring(hostname, username)
	if err == nil {
		return credential, true, nil
	}
	if !errors.Is(err, errRefreshableNotFound) {
		return gh.Credential{}, true, err
	}

	// The keyring holds no record, so fall back to the insecure configuration.
	path := refreshablePath(hostname, username)
	if _, err := c.cfg.Get(path); err != nil {
		// A missing config file yields an empty config, so c.cfg.Get returns a KeyNotFoundError rather than a real
		// read failure. Treat that as "no record anywhere" and report it as not found.
		if isKeyNotFound(err) {
			return gh.Credential{}, false, errRefreshableNotFound
		}
		return gh.Credential{}, false, err
	}
	credential, err = c.refreshableFromConfig(path)
	return credential, false, err
}

func (c *AuthConfig) refreshableFromConfig(path []string) (gh.Credential, error) {
	var record refreshableTokenRecord
	var err error
	if record.AccessToken, err = c.cfg.Get(append(path, "access_token")); err != nil {
		return gh.Credential{}, err
	}
	if record.RefreshToken, err = c.cfg.Get(append(path, "refresh_token")); err != nil {
		return gh.Credential{}, err
	}
	if record.ExpiresIn, err = c.optionalSeconds(append(path, "expires_in")); err != nil {
		return gh.Credential{}, err
	}
	if record.ExpiresAt, err = c.optionalTimestamp(append(path, "expires_at")); err != nil {
		return gh.Credential{}, err
	}
	if record.RefreshTokenExpiresIn, err = c.optionalSeconds(append(path, "refresh_token_expires_in")); err != nil {
		return gh.Credential{}, err
	}
	if record.RefreshTokenExpiresAt, err = c.optionalTimestamp(append(path, "refresh_token_expires_at")); err != nil {
		return gh.Credential{}, err
	}
	credential := record.credential(gh.TokenSourceRefreshableOAuthToken)
	return credential, validateRefreshableCredential(credential)
}

// optionalSeconds reads a raw lifetime in seconds (an ExpiresIn field) from config, returning 0 when the key is absent
// so a credential without the value is still valid.
func (c *AuthConfig) optionalSeconds(path []string) (int, error) {
	value, err := c.cfg.Get(path)
	if isKeyNotFound(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	seconds, err := strconv.Atoi(value)
	if err != nil {
		return 0, errors.New("invalid refreshable credential expiration seconds")
	}
	return seconds, nil
}

func (c *AuthConfig) optionalTimestamp(path []string) (*time.Time, error) {
	value, err := c.cfg.Get(path)
	if isKeyNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	timestamp, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return nil, errors.New("invalid refreshable credential expiration timestamp")
	}
	return &timestamp, nil
}

func refreshableFromKeyring(hostname, username string) (gh.Credential, error) {
	data, err := keyring.Get(refreshableServiceName(hostname), username)
	if err != nil {
		// A missing record and an unavailable keyring are both reported as not found
		// so resolution can fall back to a non-expiring credential, matching how non-expiring
		// keyring reads are treated during activation.
		return gh.Credential{}, errRefreshableNotFound
	}
	var record refreshableTokenRecord
	if err := json.Unmarshal([]byte(data), &record); err != nil {
		return gh.Credential{}, errors.New("invalid refreshable keyring credentials")
	}
	credential := record.credential(gh.TokenSourceKeyring)
	return credential, validateRefreshableCredential(credential)
}

func (c *AuthConfig) setRefreshableInKeyring(hostname, username string, credential gh.Credential) error {
	data, err := json.Marshal(normalizeToUTC(recordFromCredential(credential)))
	if err != nil {
		return errors.New("could not encode refreshable credentials")
	}
	return keyring.Set(refreshableServiceName(hostname), username, string(data))
}

func (c *AuthConfig) setRefreshableInConfig(hostname, username string, credential gh.Credential) {
	path := refreshablePath(hostname, username)
	// Clear the mapping first so an omitted field does not retain a stale value.
	_ = c.cfg.Remove(path)
	record := normalizeToUTC(recordFromCredential(credential))
	c.cfg.Set(append(path, "access_token"), record.AccessToken)
	c.cfg.Set(append(path, "refresh_token"), record.RefreshToken)
	if record.ExpiresIn != 0 {
		c.cfg.Set(append(path, "expires_in"), strconv.Itoa(record.ExpiresIn))
	}
	if record.ExpiresAt != nil {
		c.cfg.Set(append(path, "expires_at"), record.ExpiresAt.Format(time.RFC3339Nano))
	}
	if record.RefreshTokenExpiresIn != 0 {
		c.cfg.Set(append(path, "refresh_token_expires_in"), strconv.Itoa(record.RefreshTokenExpiresIn))
	}
	if record.RefreshTokenExpiresAt != nil {
		c.cfg.Set(append(path, "refresh_token_expires_at"), record.RefreshTokenExpiresAt.Format(time.RFC3339Nano))
	}
}

// refreshExpiryLeeway is how far ahead of a short-lived access token's expiry gh proactively refreshes it, so a token
// on the verge of expiring is renewed before it can fail an in-flight request.
const refreshExpiryLeeway = 10 * time.Minute

// accessTokenNeedsRefresh reports whether the credential's access token has expired or falls within refreshExpiryLeeway
// of expiring as of now. A credential with no recorded expiry is treated as needing a refresh, since gh cannot prove
// the token is still valid.
func accessTokenNeedsRefresh(credential gh.Credential, now time.Time) bool {
	// TESTING-ONLY HACK: remove before release, together with applyTestingExpiryOverride.
	credential = applyTestingExpiryOverride(credential)
	if credential.ExpiresAt == nil {
		return true
	}
	return !now.Add(refreshExpiryLeeway).Before(*credential.ExpiresAt)
}

// applyTestingExpiryOverride returns a copy of credential whose access token expiry is reinterpreted per the
// GH_AT_EXPIRES_IN testing override, so every refresh decision (and thus the WithRefresh accessors) gates on the
// overridden lifetime even for a credential that was acquired without the override. The lifetime is anchored at the
// token's original issuance (stored ExpiresAt minus stored ExpiresIn), so a value of n means "expires n seconds after
// it was issued". The refresh token lifetime is handled separately by refreshTokenExpired.
//
// TESTING-ONLY HACK: remove before release, together with its call site in accessTokenNeedsRefresh.
func applyTestingExpiryOverride(credential gh.Credential) gh.Credential {
	if n, ok := testingExpiryOverrideFromEnv("GH_AT_EXPIRES_IN"); ok && credential.ExpiresAt != nil {
		issuedAt := credential.ExpiresAt.Add(-time.Duration(credential.ExpiresIn) * time.Second)
		expiresAt := issuedAt.Add(time.Duration(n) * time.Second).UTC()
		credential.ExpiresIn = n
		credential.ExpiresAt = &expiresAt
	}
	return credential
}

// refreshTokenExpired reports whether the credential's refresh token has expired as of now, per the GH_RT_EXPIRES_IN
// testing override. Without the override it always returns false: nothing in this layer otherwise gates on refresh
// token expiry, because a real refresh token's expiry is realized only when the server rejects it. The lifetime is
// anchored at the refresh token's original issuance (stored RefreshTokenExpiresAt minus stored RefreshTokenExpiresIn),
// so a value of n means "expires n seconds after it was issued".
//
// TESTING-ONLY HACK: remove before release, together with its call site in refreshCredentialForUser.
func refreshTokenExpired(credential gh.Credential, now time.Time) bool {
	n, ok := testingExpiryOverrideFromEnv("GH_RT_EXPIRES_IN")
	if !ok || credential.RefreshTokenExpiresAt == nil {
		return false
	}
	issuedAt := credential.RefreshTokenExpiresAt.Add(-time.Duration(credential.RefreshTokenExpiresIn) * time.Second)
	expiresAt := issuedAt.Add(time.Duration(n) * time.Second)
	return !now.Before(expiresAt)
}

// testingExpiryOverrideFromEnv reads a token lifetime override in seconds from the named environment variable. It
// reports false when the variable is unset, empty, or not a valid integer.
//
// TESTING-ONLY HACK: remove before release, together with its call sites.
func testingExpiryOverrideFromEnv(name string) (int, bool) {
	raw := os.Getenv(name)
	if raw == "" {
		return 0, false
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	return n, true
}

// credentialFromRefreshable projects a renewed RefreshableCredential onto the universal gh.Credential shape, tagging it
// with the given source. Timestamps are cloned so the result never aliases the input's *time.Time values.
func credentialFromRefreshable(renewed gh.RefreshableCredential, source string) gh.Credential {
	return gh.Credential{
		Source:                source,
		Token:                 renewed.AccessToken,
		RefreshToken:          renewed.RefreshToken,
		ExpiresIn:             renewed.ExpiresIn,
		RefreshTokenExpiresIn: renewed.RefreshTokenExpiresIn,
		ExpiresAt:             cloneTime(renewed.ExpiresAt),
		RefreshTokenExpiresAt: cloneTime(renewed.RefreshTokenExpiresAt),
	}
}

// refreshCredentialForUser renews the given refreshable credential when its access token is expiring and persists the
// renewal to the same storage the credential already lives in. The caller supplies the already-resolved credential. It
// returns the credential to use and the outcome: RefreshStatusUnnecessary when the token is still valid,
// RefreshStatusInapplicable when no refresher is configured, RefreshStatusExpired when the server rejected the refresh
// token (in which case the user's stored credential is removed so re-authentication is required), RefreshStatusFailed
// when the exchange or persistence fails for a transient reason, and RefreshStatusDone when a renewed credential was
// stored. The re-read, token exchange, and write run as one unit under the cross-process refresh lock so concurrent gh
// processes cannot each spend the rotating refresh token.
func (c *AuthConfig) refreshCredentialForUser(hostname, username string, credential gh.Credential) (gh.Credential, gh.RefreshStatus, error) {
	if !accessTokenNeedsRefresh(credential, timeNow()) {
		return credential, gh.RefreshStatusUnnecessary, nil
	}
	if c.refresher == nil {
		return credential, gh.RefreshStatusInapplicable, nil
	}

	result := credential
	status := gh.RefreshStatusFailed
	err := c.withFreshConfigForRefresh(func() error {
		// Re-read under the lock: another gh process may have refreshed the credential while we waited for it.
		current, secure, err := c.refreshableForUser(hostname, username)
		if err != nil {
			return err
		}
		if !accessTokenNeedsRefresh(current, timeNow()) {
			result = current
			status = gh.RefreshStatusUnnecessary
			return nil
		}
		if refreshTokenExpired(current, timeNow()) {
			// TESTING-ONLY HACK: simulate the server rejecting an expired refresh token so refresh token expiry can
			// be exercised without waiting for the real lifetime. This mirrors the ErrRefreshTokenInvalid handling
			// below: wipe the now-useless stored credential and report RefreshStatusExpired. Remove before release,
			// together with refreshTokenExpired.
			result = gh.Credential{}
			status = gh.RefreshStatusExpired
			_ = c.removeCredentialForUser(hostname, username)
			return gh.ErrRefreshTokenInvalid
		}
		renewed, err := c.refresher.Refresh(current.RefreshToken, hostname)
		if err != nil {
			result = current
			// A refresh token the server rejects can never succeed on retry, so classify it distinctly from a
			// transient failure to let callers advise re-authentication instead of a pointless retry.
			if errors.Is(err, gh.ErrRefreshTokenInvalid) {
				status = gh.RefreshStatusExpired
				// The credential is now useless: we only reach a refresh because the access token is expiring or
				// already expired, and the refresh token that could renew it has just been rejected, so no future
				// call can revive it. Remove this user's stored credential so subsequent *WithRefresh callers
				// resolve no refresh token and stop re-sending a request the server will always reject, which could
				// otherwise hammer the token endpoint on every gh invocation.
				//
				// We deliberately wipe only this user's records and leave the active-user pointer and the rest of
				// the host configuration alone, rather than reusing Logout. On a multi-user host Logout would
				// silently switch the active account to a different user as a side effect of a token refresh, so a
				// subsequent request would be sent as someone else entirely. That is a surprising and potentially
				// dangerous behaviour. Leaving a dangling active user with no token is a deliberately broken state:
				// gh treats it as unauthenticated and the user must run 'gh auth login' again before using gh.
				//
				// Removal is best effort. Even if it fails we still report the rejection so the caller can classify
				// it, and we return an empty credential because the stored one is gone.
				result = gh.Credential{}
				_ = c.removeCredentialForUser(hostname, username)
			}
			return err
		}
		newCredential := credentialFromRefreshable(renewed, current.Source)
		if err := validateRefreshableCredential(newCredential); err != nil {
			result = current
			return err
		}
		if err := c.persistRefreshedCredential(hostname, username, newCredential, secure); err != nil {
			// This is a dead-end because the old credential is no longer usable (due to refresh) and the new one is
			// failed to store.
			//
			// If we return the new credential, the flow that caused this refresh attempt will proceed with the new token
			// but, subsequent flows in this process (most probably API calls) would fail because the new credential was
			// not successfully persisted. So, returning the new token would be misleading and may cause unexpected
			// failures in subsequent operations.
			//
			// We deliberately do not expose the underlying storage error for now; the caller only needs to know the
			// refresh could not be completed.
			result = current
			return fmt.Errorf("failed to persist refreshed credential; the user needs to re-authenticate via 'gh auth login': %w", err)
		}
		result = newCredential
		status = gh.RefreshStatusDone
		return nil
	})
	return result, status, err
}

// removeCredentialForUser deletes every stored credential record for the user on the host, covering both the
// refreshable and the non-expiring forms in both the keyring and the insecure config, then writes the config. It
// intentionally leaves the active-user pointer and the rest of the host configuration untouched, so it neither
// switches accounts nor removes the host; the caller is responsible for whatever locking the write requires (it is
// called under the refresh lock). Each delete is best effort so a missing record or unavailable keyring does not stop
// the others.
func (c *AuthConfig) removeCredentialForUser(hostname, username string) error {
	_ = keyring.Delete(refreshableServiceName(hostname), username)
	_ = keyring.Delete(keyringServiceName(hostname), username)
	_ = c.cfg.Remove(refreshablePath(hostname, username))
	_ = c.cfg.Remove([]string{hostsKey, hostname, usersKey, username, oauthTokenKey})
	return ghConfig.Write(c.cfg)
}

// persistRefreshedCredential writes a renewed refreshable credential back to the storage form it already lived in.
// Refreshable credentials are resolved per-user via the active user key, so there is no host-level active copy to keep
// in sync.
func (c *AuthConfig) persistRefreshedCredential(hostname, username string, credential gh.Credential, secure bool) error {
	if secure {
		if err := c.setRefreshableInKeyring(hostname, username, credential); err != nil {
			return err
		}
	} else {
		c.setRefreshableInConfig(hostname, username, credential)
	}

	return ghConfig.Write(c.cfg)
}

// refreshableTokenRecord is the on-disk and keyring JSON shape of a refreshable credential. It exists solely so the
// storage helpers can marshal and unmarshal the persisted fields; it never crosses an AuthConfig method boundary,
// where gh.Credential is the currency instead. Its json tags also match the OAuth token endpoint field names.
type refreshableTokenRecord struct {
	AccessToken           string     `json:"access_token"`
	RefreshToken          string     `json:"refresh_token"`
	ExpiresIn             int        `json:"expires_in,omitempty"`
	RefreshTokenExpiresIn int        `json:"refresh_token_expires_in,omitempty"`
	ExpiresAt             *time.Time `json:"expires_at,omitempty"`
	RefreshTokenExpiresAt *time.Time `json:"refresh_token_expires_at,omitempty"`
}

// credential projects the stored record onto the universal gh.Credential shape, tagging it with the given source
// (the access token becomes Token). Refresh metadata is carried verbatim, cloning the timestamps so the result never
// aliases the record's *time.Time values.
func (r refreshableTokenRecord) credential(source string) gh.Credential {
	return gh.Credential{
		Source:                source,
		Token:                 r.AccessToken,
		RefreshToken:          r.RefreshToken,
		ExpiresIn:             r.ExpiresIn,
		RefreshTokenExpiresIn: r.RefreshTokenExpiresIn,
		ExpiresAt:             cloneTime(r.ExpiresAt),
		RefreshTokenExpiresAt: cloneTime(r.RefreshTokenExpiresAt),
	}
}

// recordFromCredential projects a gh.Credential onto the persisted record shape, dropping the runtime-only Source and
// carrying the access token as AccessToken. It clones the timestamps so the record never aliases the credential's
// *time.Time values.
func recordFromCredential(credential gh.Credential) refreshableTokenRecord {
	return refreshableTokenRecord{
		AccessToken:           credential.Token,
		RefreshToken:          credential.RefreshToken,
		ExpiresIn:             credential.ExpiresIn,
		RefreshTokenExpiresIn: credential.RefreshTokenExpiresIn,
		ExpiresAt:             cloneTime(credential.ExpiresAt),
		RefreshTokenExpiresAt: cloneTime(credential.RefreshTokenExpiresAt),
	}
}

// cloneTime returns a pointer to a copy of t, or nil when t is nil, so callers moving timestamps between the record
// and gh.Credential never share the underlying *time.Time.
func cloneTime(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	return new(*t)
}

// normalizeToUTC returns a copy of the record with any expiration timestamps converted to UTC, so stored records use
// a single canonical timezone regardless of how the caller constructed them.
func normalizeToUTC(record refreshableTokenRecord) refreshableTokenRecord {
	if record.ExpiresAt != nil {
		utc := record.ExpiresAt.UTC()
		record.ExpiresAt = &utc
	}
	if record.RefreshTokenExpiresAt != nil {
		utc := record.RefreshTokenExpiresAt.UTC()
		record.RefreshTokenExpiresAt = &utc
	}
	return record
}

func validateRefreshableCredential(credential gh.Credential) error {
	if credential.Token == "" {
		return errors.New("access token is empty")
	}
	if credential.RefreshToken == "" {
		return errors.New("refresh token is empty")
	}
	return nil
}

func refreshablePath(hostname, username string) []string {
	path := []string{hostsKey, hostname}
	if username != "" {
		path = append(path, usersKey, username)
	}
	return append(path, refreshableTokenKey)
}

func refreshableServiceName(hostname string) string {
	// The "refreshable" marker is prefixed with an underscore, which is not a valid hostname character under the LDH
	// rule, so the segment can never collide with the plain token service of a host literally named "refreshable".
	return "gh:_refreshable:" + hostname
}

func isKeyNotFound(err error) bool {
	var keyNotFound *ghConfig.KeyNotFoundError
	return errors.As(err, &keyNotFound)
}
