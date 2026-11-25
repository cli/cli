package config

import (
	"errors"
	"os"
	"strconv"
	"time"

	"github.com/cli/cli/v2/internal/flock"
	"github.com/cli/cli/v2/internal/gh/ghtelemetry"
	ghauth "github.com/cli/go-gh/v2/pkg/auth"
)

// refreshTelemetryEventType names the telemetry event recorded once per token refresh attempt, that is, whenever an
// expiring access token makes gh take the refresh lock.
const refreshTelemetryEventType = "auth_token_refresh"

// Values of the refresh_outcome dimension of the auth_token_refresh event.
const (
	// refreshOutcomeDone means gh exchanged the refresh token and stored the renewed credential.
	refreshOutcomeDone = "done"
	// refreshOutcomeAlreadyFresh means the credential re-read under the lock no longer needed a refresh. Usually
	// another process or goroutine renewed it first, but a new login stored in the meantime looks the same.
	refreshOutcomeAlreadyFresh = "already_fresh"
	// refreshOutcomeRejectedAdopted means the server rejected the refresh token, but the store then held a different
	// credential, which gh adopted instead of removing it. See credentialRotatedByPeer.
	refreshOutcomeRejectedAdopted = "rejected_adopted"
	// refreshOutcomeRejectedRemoved means the server rejected the refresh token and gh removed the stored credential.
	refreshOutcomeRejectedRemoved = "rejected_removed"
	// refreshOutcomeRejectedRemoveFailed means the server rejected the refresh token and gh tried to remove the stored
	// credential, but writing the configuration failed, so the credential may still be on disk.
	refreshOutcomeRejectedRemoveFailed = "rejected_remove_failed"
	// refreshOutcomeRejectedKept means the server rejected the refresh token, but gh kept the stored credential
	// because it did not hold the lock.
	refreshOutcomeRejectedKept = "rejected_kept"
	// refreshOutcomeExchangeFailed means the token exchange failed for a reason other than a rejected refresh token.
	refreshOutcomeExchangeFailed = "exchange_failed"
	// refreshOutcomeInvalidResponse means the token endpoint returned a credential missing a token.
	refreshOutcomeInvalidResponse = "invalid_response"
	// refreshOutcomePersistFailed means the exchange succeeded but storing the renewed credential failed.
	refreshOutcomePersistFailed = "persist_failed"
	// refreshOutcomeSkipped means gh did not attempt the exchange because the lock file was inaccessible.
	refreshOutcomeSkipped = "skipped"
	// refreshOutcomeReadFailed means the stored credential could not be re-read under the lock.
	refreshOutcomeReadFailed = "read_failed"
	// refreshOutcomeReloadFailed means the configuration could not be reloaded from disk under the lock.
	refreshOutcomeReloadFailed = "reload_failed"
	// refreshOutcomeIncomplete means the attempt ended without reaching any other outcome, which only a panic can
	// cause.
	refreshOutcomeIncomplete = "incomplete"
)

// Values of the refresh_lock dimension of the auth_token_refresh event.
const (
	refreshLockStateHeld         = "held"
	refreshLockStateInaccessible = "inaccessible"
	refreshLockStateTimeout      = "timeout"
	refreshLockStateUnavailable  = "unavailable"
	refreshLockStateUnknown      = "unknown"
)

// Values of the storage_type dimension of the auth_token_refresh event, matching the auth_invocation event.
const (
	refreshStorageKeyring   = "keyring"
	refreshStoragePlaintext = "plaintext"
	refreshStorageUnknown   = "unknown"
)

// refreshAttempt collects the facts of one token refresh attempt for the auth_token_refresh event.
type refreshAttempt struct {
	outcome  string
	lock     string
	storage  string
	lockWait time.Duration
	exchange time.Duration
	recheck  time.Duration
}

// refreshLockState classifies the error from acquiring the refresh lock for the refresh_lock dimension.
func refreshLockState(lockErr error) string {
	switch {
	case lockErr == nil:
		return refreshLockStateHeld
	case isRefreshLockInaccessible(lockErr):
		return refreshLockStateInaccessible
	case errors.Is(lockErr, flock.ErrTimeout):
		return refreshLockStateTimeout
	default:
		return refreshLockStateUnavailable
	}
}

func refreshStorageType(secure bool) string {
	if secure {
		return refreshStorageKeyring
	}
	return refreshStoragePlaintext
}

// recordRefreshAttempt records the auth_token_refresh event for a refresh attempt against hostname. It records nothing
// when no recorder is set or when hostname is a GitHub Enterprise Server host, since enterprise usage is never
// reported. Refresh attempts are rare, at most a few per user per day, so the invocation is sampled in full.
//
// The *_set dimensions report only whether the variables that relocate gh's configuration and state directories are
// set, never their values. Processes that resolve different state directories take different refresh lock files
// while possibly sharing one credential store, which defeats the lock.
func (c *AuthConfig) recordRefreshAttempt(hostname string, attempt refreshAttempt) {
	if c.telemetry == nil || ghauth.IsEnterprise(hostname) {
		return
	}

	c.telemetry.SetSampleRate(ghtelemetry.SAMPLE_ALL)
	c.telemetry.Record(ghtelemetry.Event{
		Type: refreshTelemetryEventType,
		Dimensions: ghtelemetry.Dimensions{
			"refresh_command":     c.refreshTelemetryCommand(),
			"refresh_outcome":     attempt.outcome,
			"refresh_lock":        attempt.lock,
			"storage_type":        attempt.storage,
			"gh_config_dir_set":   envSet("GH_CONFIG_DIR"),
			"xdg_config_home_set": envSet("XDG_CONFIG_HOME"),
			"xdg_state_home_set":  envSet("XDG_STATE_HOME"),
		},
		Measures: ghtelemetry.Measures{
			"lock_wait_ms": attempt.lockWait.Milliseconds(),
			"exchange_ms":  attempt.exchange.Milliseconds(),
			"recheck_ms":   attempt.recheck.Milliseconds(),
		},
	})
}

// refreshTelemetryCommand returns the command path set by SetTelemetryCommand, or "unknown" when a refresh happens
// before the command is resolved.
func (c *AuthConfig) refreshTelemetryCommand() string {
	if command := c.telemetryCommand.Load(); command != nil {
		return *command
	}
	return "unknown"
}

func envSet(name string) string {
	return strconv.FormatBool(os.Getenv(name) != "")
}
