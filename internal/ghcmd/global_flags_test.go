package ghcmd

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadGlobalFlagSnapshotSkipsLookupWithoutEndpoint(t *testing.T) {
	t.Parallel()

	// Given startup without an opted-in development endpoint
	clientCalled := false

	// When the global snapshot is loaded
	snapshot, err := loadGlobalFlagSnapshot(t.Context(), globalFlagStartupOptions{
		externalHTTPClient: func() (*http.Client, error) {
			clientCalled = true
			return nil, nil
		},
	})

	// Then startup stays offline with disabled defaults
	require.NoError(t, err)
	assert.False(t, clientCalled, "disabled lookup must not construct an HTTP client")
	assert.False(t, snapshot.Enabled("released"), "flags default to disabled")
}

func TestLoadGlobalFlagSnapshotSkipsTelemetrySender(t *testing.T) {
	t.Parallel()

	// Given an opted-in endpoint for the internal telemetry sender
	clientCalled := false

	// When the global snapshot is loaded
	snapshot, err := loadGlobalFlagSnapshot(t.Context(), globalFlagStartupOptions{
		serverURL:       "https://flags.example",
		telemetrySender: true,
		externalHTTPClient: func() (*http.Client, error) {
			clientCalled = true
			return nil, nil
		},
	})

	// Then the internal subprocess cannot recurse into feature flag lookup
	require.NoError(t, err)
	assert.False(t, clientCalled, "telemetry delivery must not construct an HTTP client")
	assert.False(t, snapshot.Enabled("released"), "the telemetry sender uses disabled defaults")
}

func TestLoadGlobalFlagSnapshotReturnsServerSnapshot(t *testing.T) {
	t.Parallel()

	// Given an opted-in unauthenticated development endpoint
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.Header.Get("Authorization"), "startup must not attach GitHub credentials")
		_, _ = io.WriteString(w, `{"flags":{"released":true}}`)
	}))
	defer server.Close()

	// When the global snapshot is loaded
	snapshot, err := loadGlobalFlagSnapshot(t.Context(), globalFlagStartupOptions{
		serverURL: server.URL,
		cacheDir:  t.TempDir(),
		externalHTTPClient: func() (*http.Client, error) {
			return server.Client(), nil
		},
	})

	// Then callers receive the immutable server snapshot
	require.NoError(t, err)
	assert.True(t, snapshot.Enabled("released"), "the server enabled this flag globally")
}

func TestLoadGlobalFlagSnapshotReturnsClientError(t *testing.T) {
	t.Parallel()

	// Given an opted-in endpoint whose external client cannot be constructed
	wantErr := errors.New("client unavailable")

	// When the global snapshot is loaded
	snapshot, err := loadGlobalFlagSnapshot(t.Context(), globalFlagStartupOptions{
		serverURL: "https://flags.example",
		cacheDir:  t.TempDir(),
		externalHTTPClient: func() (*http.Client, error) {
			return nil, wantErr
		},
	})

	// Then the dependency error accompanies disabled defaults
	require.ErrorIs(t, err, wantErr)
	assert.False(t, snapshot.Enabled("released"), "failed client construction uses disabled defaults")
}
