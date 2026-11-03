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

	"github.com/cli/cli/v2/internal/debug"
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

// keyringDelete removes a keyring secret during the login purges. It is a package-level indirection so tests can
// simulate a delete that silently leaves the secret in place, which the keyring mock cannot express.
var keyringDelete = keyring.Delete

// refreshLockFile names the cross-process lock, in the state directory, that serializes credential-store
// read-modify-writes so they cannot race a token refresh. The name is refresh-centric because protecting the
// single-use rotating refresh token from concurrent spending is why the lock exists, but any credential mutation that
// must not race a refresh acquires it. See withFreshConfigForRefresh for the operations that take it. It lives in the
// state directory rather than the config directory because the config directory can be read-only (for example under
// nix home-manager), and a lock file is transient runtime state.
const refreshLockFile = "refresh.lock"

// refreshLockTimeout bounds how long withFreshConfigForRefresh waits for the cross-process lock before giving up and
// proceeding without it. It is long enough to outlast a normal concurrent refresh yet short enough that a stuck or
// unsupported lock cannot wedge gh indefinitely.
const refreshLockTimeout = 60 * time.Second

// flockRefreshLock acquires the cross-process refresh lock with flock, creating the lock file's directory first. It is
// the production implementation used when AuthConfig.acquireRefreshLock is nil. Creating the directory here, behind the
// seam, keeps every filesystem effect of locking out of tests that stub the seam.
func flockRefreshLock(ctx context.Context, path string) (unlock func(), err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0771); err != nil {
		return nil, err
	}
	_, unlock, err = flock.Lock(ctx, path)
	return unlock, err
}

// withFreshConfigForRefresh runs fn, a credential-store read-modify-write (a token refresh, or a logout that must not
// race one), while holding both an in-process mutex and a cross-process advisory lock, and reloads the configuration
// from disk before running fn so it observes any credential another process persisted. The mutex serializes goroutines
// sharing this AuthConfig; the file lock serializes separate gh processes. Together they prevent concurrent refreshes
// from each spending a rotating refresh token based on stale state, and ensure logout cannot be overwritten by a
// refresh that finishes after it. fn runs with both locks held and should perform the credential read and write as one
// unit.
//
// The file lock is best effort: fn still runs so a filesystem limitation never blocks the user. This tolerates both
// being unable to create the state directory that holds the lock file (for example a read-only mount) and being
// unable to acquire the lock itself (a filesystem without advisory locking, or another process holding it past
// refreshLockTimeout). fn receives lockHeld reporting whether the cross-process lock was actually acquired, so it can
// skip destructive cleanup that is only safe when peers are serialized; see refreshCredentialForUser for why cleanup
// is gated on it.
//
// The reload refreshes the shared configuration in place, so any config changes not yet written to disk are discarded.
// That is acceptable on the refresh path because mutations are persisted as they happen.
func (c *AuthConfig) withFreshConfigForRefresh(fn func(lockHeld bool) error) error {
	c.refreshMu.Lock()
	defer c.refreshMu.Unlock()

	// Best-effort cross-process lock; see the godoc. We proceed without it rather than fail the user's refresh when
	// the state directory cannot be created or the lock cannot be acquired. lockHeld then tells fn to skip cleanup
	// that is unsafe without serialized peers. The residual risk without the lock is two processes refreshing at once
	// and spending a rotating token twice.
	var lockHeld bool
	unlock := func() {}
	ctx, cancel := context.WithTimeout(context.Background(), refreshLockTimeout)
	defer cancel()

	acquire := c.acquireRefreshLock
	if acquire == nil {
		acquire = flockRefreshLock
	}

	// Put the lock in the state directory, not the config directory. The config directory can be read-only: under nix
	// home-manager and similar setups, managed config files are symlinked into the immutable nix store, so creating a
	// lock file there always fails. The state directory is XDG's location for writable runtime state, home-manager
	// leaves it writable, and gh already writes state there (state.yml, telemetry), so it is the safer bet.
	if u, err := acquire(ctx, filepath.Join(ghConfig.StateDir(), refreshLockFile)); err == nil {
		unlock = u
		lockHeld = true
	}
	defer unlock()

	if err := c.reload(); err != nil {
		return fmt.Errorf("could not reload configuration: %w", err)
	}

	return fn(lockHeld)
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
	err := c.withFreshConfigForRefresh(func(lockHeld bool) error {
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
	// refreshable record. Verify the removal: the ordinary keyring is now resolved before the refreshable credential,
	// so a silently failed delete would let a stale token outrank this login.
	if err := c.purgeNonExpiringForRefreshableLogin(hostname, username); err != nil {
		return insecureStorageUsed, err
	}

	return insecureStorageUsed, c.completeLogin(hostname, username, gitProtocol)
}

// purgeRefreshableForLogin removes any refreshable credential for the user during a non-refreshable login so the two
// credential forms cannot coexist. A surviving refreshable record outranks the freshly stored token during
// activation, so a removal that silently failed would let login report success while the old credential stays active.
//
// Rather than infer success from the delete error (which an OS keyring reports inconsistently), it verifies the
// post-condition by re-reading. Surfacing must not break environments without a working keyring, where a delete
// reports the keyring is unavailable even though nothing is stored: an unavailable keyring resolves to
// errRefreshableNotFound here, so the check stays silent exactly where a delete error would be spurious. Only a record
// that genuinely survives removal resolves without error and is surfaced.
func (c *AuthConfig) purgeRefreshableForLogin(hostname, username string) error {
	// Best-effort removal in both stores, matching loginRefreshable's symmetric cleanup.
	_ = c.cfg.Remove(refreshablePath(hostname, username))
	_ = keyringDelete(refreshableServiceName(hostname), username)

	if _, _, err := c.refreshableForUser(hostname, username); err == nil {
		return errors.New("could not remove the existing refreshable credential, which would shadow this login")
	}
	return nil
}

// purgeNonExpiringForRefreshableLogin removes any non-expiring credential for the user during a refreshable login so
// the two credential forms cannot coexist. The ordinary keyring is now resolved before the refreshable credential, so
// a surviving non-expiring token would outrank the freshly stored refreshable one; a removal that silently failed
// would let login report success while the stale token keeps winning.
//
// Like purgeRefreshableForLogin, it verifies the post-condition by re-reading rather than trusting the delete error,
// which an OS keyring reports inconsistently. An unavailable keyring makes the read fail too, which reads here as "not
// present", so the check stays silent exactly where a delete error would be spurious. Only a token that genuinely
// survives removal is surfaced.
func (c *AuthConfig) purgeNonExpiringForRefreshableLogin(hostname, username string) error {
	// Best-effort removal in both stores, matching login's symmetric cleanup.
	_ = c.cfg.Remove([]string{hostsKey, hostname, usersKey, username, oauthTokenKey})
	_ = keyringDelete(keyringServiceName(hostname), username)

	if _, err := keyring.Get(keyringServiceName(hostname), username); err == nil {
		return errors.New("could not remove the existing non-expiring credential, which would shadow this login")
	}
	if _, err := c.cfg.Get([]string{hostsKey, hostname, usersKey, username, oauthTokenKey}); err == nil {
		return errors.New("could not remove the existing non-expiring credential, which would shadow this login")
	}
	return nil
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

// refreshExpiryLeeway is how far ahead of an access token's expiry gh proactively refreshes it, so a token
// on the verge of expiring is renewed before it can fail an in-flight request.
const refreshExpiryLeeway = 10 * time.Minute

// accessTokenNeedsRefresh reports whether the credential's access token has expired or falls within refreshExpiryLeeway
// of expiring as of now. A credential with no recorded expiry is treated as needing a refresh, since gh cannot prove
// the token is still valid. A debug lifetime override is anchored at the token's original issuance.
func accessTokenNeedsRefresh(credential gh.Credential, now time.Time) bool {
	if credential.ExpiresAt == nil {
		return true
	}
	expiresAt := *credential.ExpiresAt
	if lifetime, ok := debug.Config().AccessTokenExpiresIn.Value(); ok {
		issuedAt := credential.ExpiresAt.Add(-time.Duration(credential.ExpiresIn) * time.Second)
		expiresAt = issuedAt.Add(lifetime)
	}
	return !now.Add(refreshExpiryLeeway).Before(expiresAt)
}

// simulateRefreshTokenExpired simulates refresh token expiry when a debug lifetime override is set.
// The override is anchored at issuance, computed from the stored expiry and lifetime.
// Without an override it always returns false; real expiry is detected from the server's rejection.
func simulateRefreshTokenExpired(credential gh.Credential, now time.Time) bool {
	if lifetime, ok := debug.Config().RefreshTokenExpiresIn.Value(); ok && credential.RefreshTokenExpiresAt != nil {
		issuedAt := credential.RefreshTokenExpiresAt.Add(-time.Duration(credential.RefreshTokenExpiresIn) * time.Second)
		expiresAt := issuedAt.Add(lifetime)
		return !now.Before(expiresAt)
	}
	return false
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
	err := c.withFreshConfigForRefresh(func(lockHeld bool) error {
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
		if simulateRefreshTokenExpired(current, timeNow()) {
			// Simulate server rejection under the debug override, mirroring the ErrRefreshTokenInvalid
			// handling below, including gating destructive cleanup on lockHeld.
			result = gh.Credential{}
			status = gh.RefreshStatusExpired
			if lockHeld {
				_ = c.removeCredentialForUser(hostname, username)
			}
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
				// The removal only runs when the cross-process lock was held. Without the lock a rejection is
				// ambiguous: a peer may have already spent this same single-use refresh token and be about to
				// persist the replacement. Deleting here could race that peer and discard its valid credential,
				// forcing a needless re-authentication. When the lock is unavailable we therefore leave storage
				// untouched and let the peer's write stand, while still reporting the rejection so the caller can
				// advise re-authentication. Removal is best effort even when attempted: a failure does not change
				// the reported status, and we return an empty credential because the stored one is being discarded.
				result = gh.Credential{}
				if lockHeld {
					_ = c.removeCredentialForUser(hostname, username)
				}
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
