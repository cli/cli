package config

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/cli/cli/v2/internal/flock"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/gh/ghtelemetry"
	ghmock "github.com/cli/cli/v2/internal/gh/mock"
	"github.com/stretchr/testify/require"
)

// refreshTelemetrySpy captures recorded events and the requested sample rate. The internal/telemetry spies cannot be
// used here because that package imports this one.
type refreshTelemetrySpy struct {
	events     []ghtelemetry.Event
	sampleRate int
}

func (s *refreshTelemetrySpy) Record(event ghtelemetry.Event) {
	s.events = append(s.events, event)
}

func (s *refreshTelemetrySpy) Begin(event ghtelemetry.Event) ghtelemetry.PendingEvent {
	panic("refresh telemetry must record complete events")
}

func (s *refreshTelemetrySpy) SetSampleRate(rate int) {
	s.sampleRate = rate
}

func rejectingRefresher(onRefresh func()) *ghmock.TokenRefresherMock {
	return &ghmock.TokenRefresherMock{
		RefreshFunc: func(refreshToken string, hostname string) (gh.RefreshableCredential, error) {
			if onRefresh != nil {
				onRefresh()
			}
			return gh.RefreshableCredential{}, fmt.Errorf("server rejected token: %w", gh.ErrRefreshTokenInvalid)
		},
	}
}

func TestRefreshTelemetry(t *testing.T) {
	host := "github.com"
	user := "test-user"
	future := time.Now().Add(8 * time.Hour)

	tests := []struct {
		name        string
		secure      bool
		lockErr     error
		whileLock   func(t *testing.T)
		refresher   func(t *testing.T) gh.TokenRefresher
		wantOutcome string
		wantLock    string
		wantStorage string
	}{
		{
			name:   "renewed by a peer while waiting for the lock",
			secure: true,
			whileLock: func(t *testing.T) {
				setRefreshableKeyring(t, host, user, gh.Credential{Token: "peer-access", RefreshToken: "peer-refresh", ExpiresAt: &future})
			},
			refresher:   func(*testing.T) gh.TokenRefresher { return renewingRefresher() },
			wantOutcome: "already_fresh",
			wantLock:    "held",
			wantStorage: "keyring",
		},
		{
			name:        "refreshed",
			refresher:   func(*testing.T) gh.TokenRefresher { return renewingRefresher() },
			wantOutcome: "done",
			wantLock:    "held",
			wantStorage: "plaintext",
		},
		{
			name:        "refreshed without the lock after a timeout",
			secure:      true,
			lockErr:     flock.ErrTimeout,
			refresher:   func(*testing.T) gh.TokenRefresher { return renewingRefresher() },
			wantOutcome: "done",
			wantLock:    "timeout",
			wantStorage: "keyring",
		},
		{
			name:   "rejected and adopted the peer's credential",
			secure: true,
			refresher: func(t *testing.T) gh.TokenRefresher {
				return rejectingRefresher(func() {
					setRefreshableKeyring(t, host, user, gh.Credential{Token: "peer-access", RefreshToken: "peer-refresh", ExpiresAt: &future})
				})
			},
			wantOutcome: "rejected_adopted",
			wantLock:    "held",
			wantStorage: "keyring",
		},
		{
			name:        "rejected and removed",
			refresher:   func(*testing.T) gh.TokenRefresher { return rejectingRefresher(nil) },
			wantOutcome: "rejected_removed",
			wantLock:    "held",
			wantStorage: "plaintext",
		},
		{
			name: "rejected but removal failed",
			refresher: func(t *testing.T) gh.TokenRefresher {
				return rejectingRefresher(func() {
					require.NoError(t, os.Chmod(filepath.Join(os.Getenv("GH_CONFIG_DIR"), "hosts.yml"), 0400))
				})
			},
			wantOutcome: "rejected_remove_failed",
			wantLock:    "held",
			wantStorage: "plaintext",
		},
		{
			name:        "rejected and kept without the lock",
			lockErr:     syscall.ENOLCK,
			refresher:   func(*testing.T) gh.TokenRefresher { return rejectingRefresher(nil) },
			wantOutcome: "rejected_kept",
			wantLock:    "unavailable",
			wantStorage: "plaintext",
		},
		{
			name: "exchange failed",
			refresher: func(*testing.T) gh.TokenRefresher {
				return &ghmock.TokenRefresherMock{
					RefreshFunc: func(string, string) (gh.RefreshableCredential, error) {
						return gh.RefreshableCredential{}, errors.New("token endpoint unreachable")
					},
				}
			},
			wantOutcome: "exchange_failed",
			wantLock:    "held",
			wantStorage: "plaintext",
		},
		{
			name:        "skipped because the lock file is inaccessible",
			lockErr:     &fs.PathError{Op: "open", Path: refreshLockFile, Err: syscall.EROFS},
			refresher:   func(*testing.T) gh.TokenRefresher { return renewingRefresher() },
			wantOutcome: "skipped",
			wantLock:    "inaccessible",
			wantStorage: "plaintext",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authCfg := newTestAuthConfig(t)
			loginRefreshableWithExpiry(t, authCfg, host, user, "old-access", "old-refresh", time.Now().Add(-time.Hour), tt.secure)
			authCfg.acquireRefreshLock = func(ctx context.Context, path string) (func(), error) {
				if tt.whileLock != nil {
					tt.whileLock(t)
				}
				return func() {}, tt.lockErr
			}
			authCfg.SetTokenRefresher(tt.refresher(t))
			spy := &refreshTelemetrySpy{}
			authCfg.SetTelemetryRecorder(spy)

			_, _, _ = authCfg.ActiveTokenWithRefresh(host)

			require.Equal(t, ghtelemetry.SAMPLE_ALL, spy.sampleRate)
			require.Len(t, spy.events, 1)
			event := spy.events[0]
			require.Equal(t, "auth_token_refresh", event.Type)
			require.Equal(t, tt.wantOutcome, event.Dimensions["refresh_outcome"])
			require.Equal(t, tt.wantLock, event.Dimensions["refresh_lock"])
			require.Equal(t, tt.wantStorage, event.Dimensions["storage_type"])
		})
	}
}

func TestRefreshTelemetryReportsDirectoryOverrides(t *testing.T) {
	authCfg := newTestAuthConfig(t)
	// GH_CONFIG_DIR stays pointed at the temporary directory NewIsolatedTestConfig created, since the refresh writes
	// the configuration there.
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_STATE_HOME", "/custom/state")
	loginRefreshableWithExpiry(t, authCfg, "github.com", "test-user", "old-access", "old-refresh", time.Now().Add(-time.Hour), false)
	authCfg.SetTokenRefresher(renewingRefresher())
	spy := &refreshTelemetrySpy{}
	authCfg.SetTelemetryRecorder(spy)

	_, _, err := authCfg.ActiveTokenWithRefresh("github.com")

	require.NoError(t, err)
	require.Len(t, spy.events, 1)
	require.Equal(t, "true", spy.events[0].Dimensions["gh_config_dir_set"])
	require.Equal(t, "false", spy.events[0].Dimensions["xdg_config_home_set"])
	require.Equal(t, "true", spy.events[0].Dimensions["xdg_state_home_set"])
}

func TestRefreshTelemetryReportsCommand(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    string
	}{
		{name: "set", command: "gh auth token", want: "gh auth token"},
		{name: "not set yet", want: "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authCfg := newTestAuthConfig(t)
			loginRefreshableWithExpiry(t, authCfg, "github.com", "test-user", "old-access", "old-refresh", time.Now().Add(-time.Hour), false)
			authCfg.SetTokenRefresher(renewingRefresher())
			if tt.command != "" {
				authCfg.SetTelemetryCommand(tt.command)
			}
			spy := &refreshTelemetrySpy{}
			authCfg.SetTelemetryRecorder(spy)

			_, _, err := authCfg.ActiveTokenWithRefresh("github.com")

			require.NoError(t, err)
			require.Len(t, spy.events, 1)
			require.Equal(t, tt.want, spy.events[0].Dimensions["refresh_command"])
		})
	}
}

func TestRefreshTelemetryRecordsUnfinishedAttempts(t *testing.T) {
	t.Run("when reloading the configuration fails", func(t *testing.T) {
		authCfg := newTestAuthConfig(t)
		loginRefreshableWithExpiry(t, authCfg, "github.com", "test-user", "old-access", "old-refresh", time.Now().Add(-time.Hour), false)
		authCfg.reloadConfig = func() error { return errors.New("reload failed") }
		authCfg.SetTokenRefresher(renewingRefresher())
		spy := &refreshTelemetrySpy{}
		authCfg.SetTelemetryRecorder(spy)

		_, _, err := authCfg.ActiveTokenWithRefresh("github.com")

		require.Error(t, err)
		require.Len(t, spy.events, 1)
		require.Equal(t, "reload_failed", spy.events[0].Dimensions["refresh_outcome"])
	})

	t.Run("when the refresh panics", func(t *testing.T) {
		authCfg := newTestAuthConfig(t)
		loginRefreshableWithExpiry(t, authCfg, "github.com", "test-user", "old-access", "old-refresh", time.Now().Add(-time.Hour), false)
		authCfg.SetTokenRefresher(&ghmock.TokenRefresherMock{
			RefreshFunc: func(string, string) (gh.RefreshableCredential, error) { panic("boom") },
		})
		spy := &refreshTelemetrySpy{}
		authCfg.SetTelemetryRecorder(spy)

		require.PanicsWithValue(t, "boom", func() { _, _, _ = authCfg.ActiveTokenWithRefresh("github.com") })

		require.Len(t, spy.events, 1)
		require.Equal(t, "incomplete", spy.events[0].Dimensions["refresh_outcome"])
	})
}

func TestRefreshTelemetryNotRecorded(t *testing.T) {
	t.Run("for an enterprise host", func(t *testing.T) {
		host := "ghes.example.com"
		authCfg := newTestAuthConfig(t)
		loginRefreshableWithExpiry(t, authCfg, host, "test-user", "old-access", "old-refresh", time.Now().Add(-time.Hour), false)
		authCfg.SetTokenRefresher(renewingRefresher())
		spy := &refreshTelemetrySpy{}
		authCfg.SetTelemetryRecorder(spy)

		_, status, err := authCfg.ActiveTokenWithRefresh(host)

		require.NoError(t, err)
		require.Equal(t, gh.RefreshStatusDone, status)
		require.Empty(t, spy.events)
		require.Zero(t, spy.sampleRate)
	})

	t.Run("when the token does not need a refresh", func(t *testing.T) {
		host := "github.com"
		authCfg := newTestAuthConfig(t)
		loginRefreshableWithExpiry(t, authCfg, host, "test-user", "old-access", "old-refresh", time.Now().Add(time.Hour), false)
		authCfg.SetTokenRefresher(renewingRefresher())
		spy := &refreshTelemetrySpy{}
		authCfg.SetTelemetryRecorder(spy)

		_, status, err := authCfg.ActiveTokenWithRefresh(host)

		require.NoError(t, err)
		require.Equal(t, gh.RefreshStatusUnnecessary, status)
		require.Empty(t, spy.events)
	})
}
