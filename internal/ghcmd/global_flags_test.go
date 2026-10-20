package ghcmd

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cli/cli/v2/internal/globalflags"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadGlobalFlagSnapshotSkipsLookupWithoutEndpoint(t *testing.T) {
	t.Parallel()

	// Given startup without an opted-in development endpoint
	refreshStarted := false

	// When the global snapshot is loaded
	snapshot, err := loadGlobalFlagSnapshot(globalFlagStartupOptions{
		startRefresh: func() error {
			refreshStarted = true
			return nil
		},
	})

	// Then startup stays offline with disabled defaults
	require.NoError(t, err)
	assert.False(t, refreshStarted, "disabled lookup must not start a refresh")
	assert.False(t, snapshot.Enabled("released"), "flags default to disabled")
}

func TestLoadGlobalFlagSnapshotSkipsTelemetrySender(t *testing.T) {
	t.Parallel()

	// Given an opted-in endpoint for the internal telemetry sender
	refreshStarted := false

	// When the global snapshot is loaded
	snapshot, err := loadGlobalFlagSnapshot(globalFlagStartupOptions{
		serverURL:   "https://flags.example",
		skipRefresh: true,
		startRefresh: func() error {
			refreshStarted = true
			return nil
		},
	})

	// Then the internal subprocess cannot recurse into feature flag lookup
	require.NoError(t, err)
	assert.False(t, refreshStarted, "telemetry delivery must not start a refresh")
	assert.False(t, snapshot.Enabled("released"), "the telemetry sender uses disabled defaults")
}

func TestLoadGlobalFlagSnapshotReturnsFreshCacheWithoutRefresh(t *testing.T) {
	t.Parallel()

	// Given a fresh global snapshot from an earlier refresh
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"flags":{"released":true}}`)
	}))
	defer server.Close()
	cacheDir := t.TempDir()
	require.NoError(t, globalflags.Refresh(t.Context(), server.URL, cacheDir, server.Client()))
	refreshStarted := false

	// When the next invocation loads its global snapshot
	snapshot, err := loadGlobalFlagSnapshot(globalFlagStartupOptions{
		serverURL: server.URL,
		cacheDir:  cacheDir,
		startRefresh: func() error {
			refreshStarted = true
			return nil
		},
	})

	// Then it returns immediately without starting another refresh
	require.NoError(t, err)
	assert.False(t, refreshStarted, "fresh cached flags need no refresh")
	assert.True(t, snapshot.Enabled("released"), "the cached flag applies to this invocation")
}

func TestLoadGlobalFlagSnapshotStartsRefreshForMissingCache(t *testing.T) {
	t.Parallel()

	// Given an opted-in endpoint without a cached snapshot
	refreshStarted := false

	// When startup loads its global snapshot
	snapshot, err := loadGlobalFlagSnapshot(globalFlagStartupOptions{
		serverURL: "https://flags.example",
		cacheDir:  t.TempDir(),
		startRefresh: func() error {
			refreshStarted = true
			return nil
		},
	})

	// Then it immediately uses defaults while refreshing for the next invocation
	require.NoError(t, err)
	assert.True(t, refreshStarted, "a missing cache must start a detached refresh")
	assert.False(t, snapshot.Enabled("released"), "the current invocation uses disabled defaults")
}

func TestLoadGlobalFlagSnapshotReturnsRefreshStartError(t *testing.T) {
	t.Parallel()

	// Given an opted-in endpoint whose refresh worker cannot be started
	wantErr := errors.New("start refresh")

	// When the global snapshot is loaded
	snapshot, err := loadGlobalFlagSnapshot(globalFlagStartupOptions{
		serverURL: "https://flags.example",
		cacheDir:  t.TempDir(),
		startRefresh: func() error {
			return wantErr
		},
	})

	// Then the start error accompanies immediately available defaults
	require.ErrorIs(t, err, wantErr)
	assert.False(t, snapshot.Enabled("released"), "failed refresh startup uses disabled defaults")
}
