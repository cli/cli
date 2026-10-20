package globalflags

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefreshUsesProvisionalTwirpContract(t *testing.T) {
	t.Parallel()

	// Given an unauthenticated global flag service
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/twirp/github.cli.globalflags.v1.GlobalFeatureFlags/GetFlags", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Empty(t, r.Header.Get("Authorization"))
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		assert.JSONEq(t, `{}`, string(body))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"flags":{"released":true,"unreleased":false}}`)
	}))
	defer server.Close()

	// When the cache worker refreshes from the proposed API
	cacheDir := t.TempDir()
	err := Refresh(t.Context(), server.URL, cacheDir, server.Client())

	// Then the next invocation can distinguish enabled flags without user context
	require.NoError(t, err)
	snapshot, fresh, err := FromCache(server.URL, cacheDir)
	require.NoError(t, err)
	assert.True(t, fresh)
	assert.True(t, snapshot.Enabled("released"), "the server enabled this flag globally")
	assert.False(t, snapshot.Enabled("unreleased"), "the server disabled this flag globally")
	assert.False(t, snapshot.Enabled("unknown"), "unknown flags default to disabled")
}

func TestSnapshotOwnsItsValues(t *testing.T) {
	t.Parallel()

	// Given flags supplied by a constructor's caller
	values := map[string]bool{"released": true}
	snapshot := NewSnapshot(values)

	// When the caller changes the original map
	values["released"] = false
	values["later"] = true

	// Then the snapshot remains stable, including when copied with a Factory
	copied := snapshot
	assert.True(t, copied.Enabled("released"), "an existing snapshot must retain its values")
	assert.False(t, copied.Enabled("later"), "an existing snapshot must not acquire new values")
}

func TestFromCacheWithoutEndpointIsOffline(t *testing.T) {
	t.Parallel()

	// When startup has no opted-in development endpoint
	snapshot, fresh, err := FromCache("", "")

	// Then defaults require neither the network nor a cache
	require.NoError(t, err)
	assert.True(t, fresh, "disabled lookup must not request a refresh")
	assert.False(t, snapshot.Enabled("released"), "flags default to disabled before activation")
}

func TestFromCacheWithoutEntryRequestsRefresh(t *testing.T) {
	t.Parallel()

	// Given an opted-in endpoint without a cached snapshot
	// When startup reads global flags
	snapshot, fresh, err := FromCache("https://flags.example", t.TempDir())

	// Then defaults are immediately available and a refresh is requested
	require.NoError(t, err)
	assert.False(t, fresh, "a missing cache must request an asynchronous refresh")
	assert.False(t, snapshot.Enabled("released"), "a missing cache uses disabled defaults")
}

func TestRefreshWritesSnapshotForNextInvocation(t *testing.T) {
	t.Parallel()

	// Given an unauthenticated global flag service and an empty cache
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.Header.Get("Authorization"))
		_, _ = io.WriteString(w, `{"flags":{"released":true}}`)
	}))
	defer server.Close()
	cacheDir := t.TempDir()

	// When the refresh worker fetches the complete snapshot
	err := Refresh(t.Context(), server.URL, cacheDir, server.Client())

	// Then the next invocation reads it without contacting the service
	require.NoError(t, err)
	snapshot, fresh, err := FromCache(server.URL, cacheDir)
	require.NoError(t, err)
	assert.True(t, fresh, "a completed refresh produces a fresh cache")
	assert.True(t, snapshot.Enabled("released"), "the refreshed flag applies to the next invocation")
}

func TestFromCacheRejectsAmbiguousServerURLsBeforeIO(t *testing.T) {
	t.Parallel()
	for _, endpoint := range []string{
		"flags.example",
		"ftp://flags.example",
		"https://user:password@flags.example",
		"https://flags.example?token=secret",
		"https://flags.example?",
		"https://flags.example#fragment",
	} {
		t.Run(endpoint, func(t *testing.T) {
			t.Parallel()

			// When development lookup uses an ambiguous or authenticated URL
			snapshot, fresh, err := FromCache(endpoint, t.TempDir())

			// Then invalid configuration is reported without contacting any server
			require.ErrorContains(t, err, "global flag server URL")
			assert.False(t, fresh)
			assert.False(t, snapshot.Enabled("released"), "invalid endpoints must use default flag values")
		})
	}
}

func TestRefreshDoesNotFollowRedirects(t *testing.T) {
	t.Parallel()

	// Given a development service redirecting outside its configured endpoint
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.WithHost(httpmock.MatchAny, "flags.example"),
		httpmock.WithHeader(httpmock.StatusStringResponse(302, ""), "Location", "https://elsewhere.example/flags"))
	reg.Exclude(t, func(req *http.Request) bool { return req.URL.Host == "elsewhere.example" })
	client := &http.Client{Transport: reg}

	// When flags are fetched
	err := Refresh(t.Context(), "https://flags.example", t.TempDir(), client)

	// Then the redirect is reported rather than becoming part of this cache's identity
	require.ErrorContains(t, err, "HTTP 302")
	assert.Nil(t, client.CheckRedirect, "the shared client must not be reconfigured")
}

func TestRefreshAcceptsResponseAtOneMiBSizeLimit(t *testing.T) {
	t.Parallel()

	// Given a valid response exactly at the supported size limit
	body := `{"flags":{"released":true}}`
	body += strings.Repeat(" ", (1<<20)-len(body))
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.MatchAny, httpmock.StringResponse(body))

	// When the global snapshot is refreshed
	cacheDir := t.TempDir()
	err := Refresh(t.Context(), "https://flags.example", cacheDir, &http.Client{Transport: reg})

	// Then the inclusive size boundary permits its flag values
	require.NoError(t, err)
	snapshot, fresh, err := FromCache("https://flags.example", cacheDir)
	require.NoError(t, err)
	assert.True(t, fresh)
	assert.True(t, snapshot.Enabled("released"), "a complete response at the limit is still valid")
}

func TestRefreshCachesOnlyValidatedFlagValues(t *testing.T) {
	t.Parallel()

	// Given a valid response with irrelevant top-level data
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.MatchAny, httpmock.StringResponse(`{"flags":{"released":true},"ignored":"large service metadata"}`))
	cacheDir := t.TempDir()

	// When the worker persists the response
	err := Refresh(t.Context(), "https://flags.example", cacheDir, &http.Client{Transport: reg})

	// Then synchronous startup reads a compact normalized snapshot
	require.NoError(t, err)
	data, err := os.ReadFile(cachePath(cacheDir))
	require.NoError(t, err)
	assert.NotContains(t, string(data), "large service metadata", "cache should not preserve irrelevant response data")
	snapshot, fresh, err := FromCache("https://flags.example", cacheDir)
	require.NoError(t, err)
	assert.True(t, fresh, "normalized cache should remain a fresh snapshot")
	assert.True(t, snapshot.Enabled("released"), "normalization must preserve validated flag values")
}
