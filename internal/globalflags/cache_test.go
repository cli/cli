package globalflags

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCacheFreshnessAndSnapshotStability(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		cacheDir := t.TempDir()
		reg := &httpmock.Registry{}
		defer reg.Verify(t)
		reg.Register(httpmock.MatchAny, httpmock.StringResponse(`{"flags":{"released":true}}`))
		reg.Register(httpmock.MatchAny, httpmock.StringResponse(`{"flags":{}}`))
		client := &http.Client{Transport: reg}
		require.NoError(t, Refresh(t.Context(), "https://flags.example", cacheDir, client))

		original, fresh, err := FromCache("https://flags.example", cacheDir)
		require.NoError(t, err)
		assert.True(t, fresh)
		assert.True(t, original.Enabled("released"))

		time.Sleep(30 * time.Minute)
		stale, fresh, err := FromCache("https://flags.example", cacheDir)
		require.NoError(t, err)
		assert.False(t, fresh)
		assert.True(t, stale.Enabled("released"))

		require.NoError(t, Refresh(t.Context(), "https://flags.example", cacheDir, client))
		refreshed, fresh, err := FromCache("https://flags.example", cacheDir)
		require.NoError(t, err)
		assert.True(t, fresh)
		assert.False(t, refreshed.Enabled("released"), "removed flags must not survive a successful refresh")
		assert.True(t, original.Enabled("released"), "running invocations retain their original snapshot")
	})
}

func TestBeginRefreshThrottlesRepeatedAttemptsUntilRetryInterval(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		// Given one refresh attempt for an endpoint
		cacheDir := t.TempDir()
		first, err := BeginRefresh("https://flags.example", cacheDir)
		require.NoError(t, err)
		require.NotNil(t, first)

		// When the same endpoint requests another refresh before and after the retry interval
		throttled, err := BeginRefresh("https://flags.example", cacheDir)
		require.NoError(t, err)
		time.Sleep(refreshRetryInterval)
		retry, err := BeginRefresh("https://flags.example", cacheDir)

		// Then only the later request owns another worker launch
		require.NoError(t, err)
		assert.Nil(t, throttled, "repeated failures must not launch one worker per invocation")
		assert.NotNil(t, retry, "refresh should retry after the cooldown")
	})
}

func TestBeginRefreshDoesNotThrottleAnotherEndpoint(t *testing.T) {
	t.Parallel()

	// Given one development endpoint has recently requested a refresh
	cacheDir := t.TempDir()
	first, err := BeginRefresh("https://first.example", cacheDir)
	require.NoError(t, err)
	require.NotNil(t, first)

	// When lookup switches to another endpoint override
	second, err := BeginRefresh("https://second.example", cacheDir)

	// Then the first endpoint's failure does not suppress the new context
	require.NoError(t, err)
	assert.NotNil(t, second, "endpoint overrides need independent retry identity")
}

func TestCancelledRefreshRequestCanRetryImmediately(t *testing.T) {
	t.Parallel()

	// Given a refresh request whose worker process never started
	cacheDir := t.TempDir()
	request, err := BeginRefresh("https://flags.example", cacheDir)
	require.NoError(t, err)
	require.NotNil(t, request)
	require.NoError(t, request.Cancel())

	// When a later invocation requests the refresh
	retry, err := BeginRefresh("https://flags.example", cacheDir)

	// Then it can launch a replacement immediately
	require.NoError(t, err)
	assert.NotNil(t, retry, "process launch failures must not consume the retry interval")
}

func TestRefreshFailureIsReportedDuringCooldown(t *testing.T) {
	t.Parallel()

	// Given a detached refresh that failed after its worker started
	cacheDir := t.TempDir()
	request, err := BeginRefresh("https://flags.example", cacheDir)
	require.NoError(t, err)
	require.NotNil(t, request)
	require.NoError(t, RecordRefreshFailure("https://flags.example", cacheDir, errors.New("service unavailable")))

	// When another invocation checks whether to start a worker
	retry, err := BeginRefresh("https://flags.example", cacheDir)

	// Then it avoids another process while preserving a useful debug diagnostic
	assert.Nil(t, retry, "failed refreshes should remain throttled")
	require.ErrorContains(t, err, "last global flag refresh failed: service unavailable")
}

func TestRefreshTimesOutWithoutErasingStaleSnapshot(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
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
		require.NoError(t, Refresh(t.Context(), "https://flags.example", cacheDir, client))
		time.Sleep(30 * time.Minute)

		start := time.Now()
		err := Refresh(t.Context(), "https://flags.example", cacheDir, client)

		require.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Equal(t, 2*time.Second, time.Since(start))
		snapshot, fresh, err := FromCache("https://flags.example", cacheDir)
		require.NoError(t, err)
		assert.False(t, fresh)
		assert.True(t, snapshot.Enabled("released"), "a slow server must not erase the last valid snapshot")
	})
}

func TestRefreshRejectsInvalidResponseWithoutPoisoningCache(t *testing.T) {
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
				cacheDir := t.TempDir()
				reg := &httpmock.Registry{}
				defer reg.Verify(t)
				reg.Register(httpmock.MatchAny, httpmock.StringResponse(`{"flags":{"released":true}}`))
				reg.Register(httpmock.MatchAny, httpmock.StatusStringResponse(tt.status, tt.body))
				reg.Register(httpmock.MatchAny, httpmock.StringResponse(`{"flags":{"recovered":true}}`))
				client := &http.Client{Transport: reg}
				require.NoError(t, Refresh(t.Context(), "https://flags.example", cacheDir, client))
				time.Sleep(30 * time.Minute)

				require.Error(t, Refresh(t.Context(), "https://flags.example", cacheDir, client))
				snapshot, fresh, err := FromCache("https://flags.example", cacheDir)
				require.NoError(t, err)
				assert.False(t, fresh)
				assert.True(t, snapshot.Enabled("released"), "invalid responses must not replace valid flags")

				require.NoError(t, Refresh(t.Context(), "https://flags.example", cacheDir, client))
				recovered, fresh, err := FromCache("https://flags.example", cacheDir)
				require.NoError(t, err)
				assert.True(t, fresh)
				assert.True(t, recovered.Enabled("recovered"))
			})
		})
	}
}

func TestCacheIsolatesDevelopmentEndpoints(t *testing.T) {
	t.Parallel()
	cacheDir := t.TempDir()
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.WithHost(httpmock.MatchAny, "first.example"),
		httpmock.StringResponse(`{"flags":{"first":true}}`))
	reg.Register(httpmock.WithHost(httpmock.MatchAny, "second.example"),
		httpmock.StringResponse(`{"flags":{"second":true}}`))
	client := &http.Client{Transport: reg}
	require.NoError(t, Refresh(t.Context(), "https://first.example", cacheDir, client))

	second, fresh, err := FromCache("https://second.example", cacheDir)
	require.NoError(t, err)
	assert.False(t, fresh)
	assert.False(t, second.Enabled("first"), "fallback must not cross endpoint identities")
	disabled, fresh, err := FromCache("", cacheDir)
	require.NoError(t, err)
	assert.True(t, fresh)
	assert.False(t, disabled.Enabled("first"), "the default context must never read development flags")

	require.NoError(t, Refresh(t.Context(), "https://second.example", cacheDir, client))
	second, fresh, err = FromCache("https://second.example", cacheDir)
	require.NoError(t, err)
	assert.True(t, fresh)
	assert.True(t, second.Enabled("second"))
	assert.False(t, second.Enabled("first"))
	entries, err := os.ReadDir(filepath.Join(cacheDir, "global-feature-flags"))
	require.NoError(t, err)
	assert.Len(t, entries, 2, "the bounded development cache contains one snapshot and one lock file")
}

func TestRefreshReportsUnwritableCache(t *testing.T) {
	t.Parallel()
	cacheDir := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(cacheDir, nil, 0o600))
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	err := Refresh(t.Context(), "https://flags.example", cacheDir, &http.Client{Transport: reg})

	var pathErr *os.PathError
	require.ErrorAs(t, err, &pathErr)
}

func TestInvalidCacheCanBeRepairedByRefresh(t *testing.T) {
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
			cacheDir := t.TempDir()
			flagsDir := filepath.Join(cacheDir, "global-feature-flags")
			require.NoError(t, os.Mkdir(flagsDir, 0o700))
			require.NoError(t, os.WriteFile(filepath.Join(flagsDir, "development-v1.json"), []byte(tt.data), 0o600))

			snapshot, fresh, err := FromCache("https://flags.example", cacheDir)
			require.ErrorContains(t, err, tt.err)
			assert.False(t, fresh)
			assert.False(t, snapshot.Enabled("untrusted"))

			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(httpmock.MatchAny, httpmock.StringResponse(`{"flags":{"recovered":true}}`))
			require.NoError(t, Refresh(t.Context(), "https://flags.example", cacheDir, &http.Client{Transport: reg}))
			repaired, fresh, err := FromCache("https://flags.example", cacheDir)
			require.NoError(t, err)
			assert.True(t, fresh)
			assert.True(t, repaired.Enabled("recovered"))
		})
	}
}

func TestRefreshDeduplicatesConcurrentWorkers(t *testing.T) {
	t.Parallel()
	var requests atomic.Int32
	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		close(requestStarted)
		<-releaseRequest
		_, _ = io.WriteString(w, `{"flags":{"released":true}}`)
	}))
	defer server.Close()
	cacheDir := t.TempDir()

	done := make(chan error)
	go func() {
		done <- Refresh(t.Context(), server.URL, cacheDir, server.Client())
	}()
	<-requestStarted

	require.NoError(t, Refresh(t.Context(), server.URL, cacheDir, server.Client()))
	assert.Equal(t, int32(1), requests.Load(), "a second worker must not duplicate the in-flight fetch")
	close(releaseRequest)
	require.NoError(t, <-done)
}

func TestRefreshContinuesWhenLockingIsUnsupported(t *testing.T) {
	t.Parallel()
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.MatchAny, httpmock.StringResponse(`{"flags":{"released":true}}`))
	unsupported := errors.New("unsupported locking")

	err := refresh(
		t.Context(),
		"https://flags.example",
		t.TempDir(),
		&http.Client{Transport: reg},
		func(string) (*os.File, func(), error) { return nil, nil, unsupported },
		func(err error) bool { return errors.Is(err, unsupported) },
	)

	require.NoError(t, err)
}

func TestRefreshReportsUnexpectedLockFailure(t *testing.T) {
	t.Parallel()
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	lockErr := errors.New("lock storage failed")

	err := refresh(
		t.Context(),
		"https://flags.example",
		t.TempDir(),
		&http.Client{Transport: reg},
		func(string) (*os.File, func(), error) { return nil, nil, lockErr },
		func(error) bool { return false },
	)

	require.ErrorIs(t, err, lockErr)
	require.ErrorContains(t, err, "lock global flag refresh")
}

type cacheResult struct {
	snapshot Snapshot
	err      error
}

func TestConcurrentRefreshAndCacheReadsOnlyObserveCompleteSnapshots(t *testing.T) {
	t.Parallel()
	cacheDir := t.TempDir()
	body := `{"flags":{"released":true,"` + strings.Repeat("padding", 20000) + `":false}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()
	require.NoError(t, Refresh(t.Context(), server.URL, cacheDir, server.Client()))

	results := make(chan cacheResult, 64)
	start := make(chan struct{})
	var workers sync.WaitGroup
	for range 8 {
		workers.Go(func() {
			<-start
			for range 4 {
				results <- resultFromCache(server.URL, cacheDir)
			}
		})
		workers.Go(func() {
			<-start
			for range 4 {
				_ = Refresh(t.Context(), server.URL, cacheDir, server.Client())
			}
		})
	}
	close(start)
	workers.Wait()
	close(results)

	for result := range results {
		require.NoError(t, result.err)
		assert.True(t, result.snapshot.Enabled("released"), "readers must not observe a partial cache write")
	}
}

func resultFromCache(serverURL, cacheDir string) cacheResult {
	snapshot, _, err := FromCache(serverURL, cacheDir)
	return cacheResult{snapshot: snapshot, err: err}
}
