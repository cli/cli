package globalflags

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadReusesSnapshotUntilThirtyMinuteExpiry(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		// Given a cached global snapshot and a server that subsequently removes its flag
		cacheDir := t.TempDir()
		reg := &httpmock.Registry{}
		defer reg.Verify(t)
		reg.Register(httpmock.MatchAny, httpmock.StringResponse(`{"flags":{"released":true}}`))
		reg.Register(httpmock.MatchAny, httpmock.StringResponse(`{"flags":{}}`))
		client := &http.Client{Transport: reg}
		original, err := Load(t.Context(), "https://flags.example", cacheDir, client)
		require.NoError(t, err)

		// When another invocation starts just before expiry
		time.Sleep(30*time.Minute - time.Nanosecond)
		fresh, err := Load(t.Context(), "https://flags.example", cacheDir, client)

		// Then it reuses the cache without contacting the server
		require.NoError(t, err)
		assert.True(t, fresh.Enabled("released"), "the cached snapshot is fresh for thirty minutes")

		// When the cache reaches its expiry boundary
		time.Sleep(time.Nanosecond)
		refreshed, err := Load(t.Context(), "https://flags.example", cacheDir, client)

		// Then the new response replaces the cache without changing an earlier snapshot
		require.NoError(t, err)
		assert.False(t, refreshed.Enabled("released"), "removed flags must not survive a successful refresh")
		assert.True(t, original.Enabled("released"), "running invocations retain their original snapshot")
	})
}

func TestLoadFallsBackToStaleSnapshotAfterTwoSeconds(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		// Given a stale snapshot and a server taking ten seconds to answer
		cacheDir := t.TempDir()
		reg := &httpmock.Registry{}
		defer reg.Verify(t)
		reg.Register(httpmock.MatchAny, httpmock.StringResponse(`{"flags":{"released":true}}`))
		reg.Register(httpmock.MatchAny, func(req *http.Request) (*http.Response, error) {
			select {
			case <-req.Context().Done():
				return nil, req.Context().Err()
			case <-time.After(10 * time.Second):
				return httpmock.StringResponse(`{"flags":{}}`)(req)
			}
		})
		client := &http.Client{Transport: reg}
		_, err := Load(t.Context(), "https://flags.example", cacheDir, client)
		require.NoError(t, err)
		time.Sleep(30 * time.Minute)

		// When startup refreshes the flags
		start := time.Now()
		snapshot, err := Load(t.Context(), "https://flags.example", cacheDir, client)

		// Then the request is cancelled at two seconds and stale values remain usable
		require.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Equal(t, 2*time.Second, time.Since(start))
		assert.True(t, snapshot.Enabled("released"), "a slow server must not erase the last valid snapshot")
	})
}

func TestLoadRejectsInvalidRefreshWithoutPoisoningCache(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{name: "missing flags", status: 200, body: `{}`},
		{name: "null flags", status: 200, body: `{"flags":null}`},
		{name: "null value", status: 200, body: `{"flags":{"released":false,"broken":null}}`},
		{name: "wrong value type", status: 200, body: `{"flags":{"released":"true"}}`},
		{name: "blank name", status: 200, body: `{"flags":{" ":true}}`},
		{name: "truncated response", status: 200, body: `{"flags":{"released":false}`},
		{name: "trailing JSON", status: 200, body: `{"flags":{}} {}`},
		{name: "wrong container", status: 200, body: `{"flags":[]}`},
		{name: "null response", status: 200, body: `null`},
		{name: "oversized response", status: 200, body: `{"flags":{}}` + strings.Repeat(" ", (1<<20)-len(`{"flags":{}}`)+1)},
		{name: "server error", status: 503, body: `{"flags":{}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				// Given a stale snapshot and an invalid server response
				cacheDir := t.TempDir()
				reg := &httpmock.Registry{}
				defer reg.Verify(t)
				reg.Register(httpmock.MatchAny, httpmock.StringResponse(`{"flags":{"released":true}}`))
				reg.Register(httpmock.MatchAny, httpmock.StatusStringResponse(tt.status, tt.body))
				client := &http.Client{Transport: reg}
				_, err := Load(t.Context(), "https://flags.example", cacheDir, client)
				require.NoError(t, err)
				time.Sleep(30 * time.Minute)

				// When the response is rejected
				snapshot, err := Load(t.Context(), "https://flags.example", cacheDir, client)

				// Then the entire old snapshot is retained and the next invocation can retry
				require.Error(t, err)
				assert.True(t, snapshot.Enabled("released"), "invalid responses must not replace valid flags")
				reg.Register(httpmock.MatchAny, httpmock.StringResponse(`{"flags":{"recovered":true}}`))
				recovered, err := Load(t.Context(), "https://flags.example", cacheDir, client)
				require.NoError(t, err)
				assert.True(t, recovered.Enabled("recovered"), "a rejected refresh must not mark the cache fresh")
			})
		})
	}
}

func TestLoadIsolatesEndpointsInOneDevelopmentCacheSlot(t *testing.T) {
	t.Parallel()

	// Given cached flags belonging to one development server
	cacheDir := t.TempDir()
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.WithHost(httpmock.MatchAny, "first.example"),
		httpmock.StringResponse(`{"flags":{"first":true}}`))
	reg.Register(httpmock.WithHost(httpmock.MatchAny, "second.example"),
		httpmock.StatusStringResponse(503, ""))
	reg.Register(httpmock.WithHost(httpmock.MatchAny, "second.example"),
		httpmock.StringResponse(`{"flags":{"second":true}}`))
	client := &http.Client{Transport: reg}
	_, err := Load(t.Context(), "https://first.example", cacheDir, client)
	require.NoError(t, err)

	// When another endpoint is unavailable, or lookup is disabled
	failed, err := Load(t.Context(), "https://second.example", cacheDir, client)
	require.ErrorContains(t, err, "HTTP 503")
	disabled, err := Load(t.Context(), "", cacheDir, client)

	// Then neither can inherit the first server's flags
	require.NoError(t, err)
	assert.False(t, failed.Enabled("first"), "fallback must not cross endpoint identities")
	assert.False(t, disabled.Enabled("first"), "the default context must never read development flags")

	// When the second endpoint recovers
	recovered, err := Load(t.Context(), "https://second.example", cacheDir, client)

	// Then it replaces the single development slot rather than accumulating per-endpoint files
	require.NoError(t, err)
	assert.True(t, recovered.Enabled("second"), "the new endpoint supplies its own snapshot")
	assert.False(t, recovered.Enabled("first"), "snapshots from different endpoints must never be merged")
	entries, err := os.ReadDir(filepath.Join(cacheDir, "global-feature-flags"))
	require.NoError(t, err)
	assert.Len(t, entries, 1, "completed refreshes leave only one bounded development cache file")
}

func TestLoadReturnsServerFlagsWhenCacheIsUnwritable(t *testing.T) {
	t.Parallel()

	// Given a file where the cache directory should be
	cacheDir := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(cacheDir, nil, 0600))
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.MatchAny, httpmock.StringResponse(`{"flags":{"released":true}}`))

	// When the server successfully returns flags
	snapshot, err := Load(t.Context(), "https://flags.example", cacheDir, &http.Client{Transport: reg})

	// Then a distinguishable filesystem error accompanies the usable server snapshot
	var pathErr *os.PathError
	require.ErrorAs(t, err, &pathErr)
	assert.True(t, snapshot.Enabled("released"), "cache persistence is not a prerequisite for obtaining flags")
}

func TestLoadReportsAndRepairsInvalidCache(t *testing.T) {
	t.Parallel()
	validEntry := fmt.Sprintf(`{"endpoint":"https://flags.example/twirp/github.cli.globalflags.v1.GlobalFeatureFlags/GetFlags","fetched_at":%q,"response":{"flags":{"untrusted":true}}}`,
		time.Now().UTC().Format(time.RFC3339Nano))
	tests := []struct {
		name string
		data string
		err  string
	}{
		{name: "truncated JSON", data: `{"endpoint":`, err: "decode global flag cache"},
		{name: "missing endpoint", data: `{}`, err: "no endpoint"},
		{name: "future timestamp", data: `{"endpoint":"https://flags.example/twirp/github.cli.globalflags.v1.GlobalFeatureFlags/GetFlags","fetched_at":"2999-01-01T00:00:00Z","response":{"flags":{"untrusted":true}}}`, err: "fetch time"},
		{name: "missing timestamp", data: `{"endpoint":"https://flags.example/twirp/github.cli.globalflags.v1.GlobalFeatureFlags/GetFlags","response":{"flags":{}}}`, err: "fetch time"},
		{name: "oversized cache", data: validEntry + strings.Repeat(" ", 2<<20), err: "byte limit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Given a corrupt development cache
			cacheDir := t.TempDir()
			flagsDir := filepath.Join(cacheDir, "global-feature-flags")
			require.NoError(t, os.Mkdir(flagsDir, 0700))
			require.NoError(t, os.WriteFile(filepath.Join(flagsDir, "development-v1.json"), []byte(tt.data), 0600))
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(httpmock.MatchAny, httpmock.StringResponse(`{"flags":{"recovered":true}}`))
			client := &http.Client{Transport: reg}

			// When flags are loaded
			snapshot, err := Load(t.Context(), "https://flags.example", cacheDir, client)

			// Then corruption is reported, but valid server flags repair the cache
			require.ErrorContains(t, err, tt.err)
			assert.True(t, snapshot.Enabled("recovered"), "invalid storage must not prevent a server lookup")
			assert.False(t, snapshot.Enabled("untrusted"), "invalid cache entries must not supply flag values")
			repaired, err := Load(t.Context(), "https://flags.example", cacheDir, client)
			require.NoError(t, err)
			assert.True(t, repaired.Enabled("recovered"), "later invocations use the repaired cache")
		})
	}
}

func TestConcurrentLoadsOnlyObserveCompleteSnapshots(t *testing.T) {
	t.Parallel()

	// Given several invocations sharing a cache and a sizable server snapshot
	cacheDir := t.TempDir()
	body := `{"flags":{"released":true,"` + strings.Repeat("padding", 20000) + `":false}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()
	client := server.Client()
	type result struct {
		snapshot Snapshot
		err      error
	}
	results := make(chan result, 64)
	start := make(chan struct{})
	var workers sync.WaitGroup

	// When startup reads and refreshes race with each other
	for range 16 {
		workers.Go(func() {
			<-start
			for range 4 {
				snapshot, err := Load(t.Context(), server.URL, cacheDir, client)
				results <- result{snapshot, err}
			}
		})
	}
	close(start)
	workers.Wait()
	close(results)

	// Then every invocation gets complete values, including a later offline invocation
	for result := range results {
		require.NoError(t, result.err)
		assert.True(t, result.snapshot.Enabled("released"), "readers must not observe a partial cache write")
	}
	server.Close()
	cached, err := Load(t.Context(), server.URL, cacheDir, client)
	require.NoError(t, err)
	assert.True(t, cached.Enabled("released"), "a complete snapshot must remain persisted after concurrent refreshes")
}
