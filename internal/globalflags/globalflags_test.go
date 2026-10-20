package globalflags

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadGlobalSnapshotFromProvisionalTwirpServer(t *testing.T) {
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

	// When a snapshot is loaded from the proposed API
	snapshot, err := Load(t.Context(), server.URL, t.TempDir(), server.Client())

	// Then callers can distinguish enabled flags without any user context
	require.NoError(t, err)
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

func TestLoadWithoutEndpointIsOffline(t *testing.T) {
	t.Parallel()

	// Given no opted-in development endpoint or usable dependencies
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	// When flags are loaded
	snapshot, err := Load(ctx, "", "", nil)

	// Then defaults require neither the network nor a cache
	require.NoError(t, err)
	assert.False(t, snapshot.Enabled("released"), "flags default to disabled before activation")
}

func TestLoadRejectsAmbiguousServerURLsBeforeIO(t *testing.T) {
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

			// Given an endpoint which is not an unambiguous unauthenticated HTTP base URL
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Exclude(t, httpmock.MatchAny)

			// When the development lookup is configured
			snapshot, err := Load(t.Context(), endpoint, t.TempDir(), &http.Client{Transport: reg})

			// Then invalid configuration is reported without contacting any server
			require.ErrorContains(t, err, "global flag server URL")
			assert.False(t, snapshot.Enabled("released"), "invalid endpoints must use default flag values")
		})
	}
}

func TestLoadDoesNotFollowRedirects(t *testing.T) {
	t.Parallel()

	// Given a development service redirecting outside its configured endpoint
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.WithHost(httpmock.MatchAny, "flags.example"),
		httpmock.WithHeader(httpmock.StatusStringResponse(302, ""), "Location", "https://elsewhere.example/flags"))
	reg.Exclude(t, func(req *http.Request) bool { return req.URL.Host == "elsewhere.example" })
	client := &http.Client{Transport: reg}

	// When flags are fetched
	snapshot, err := Load(t.Context(), "https://flags.example", t.TempDir(), client)

	// Then the redirect is reported rather than becoming part of this cache's identity
	require.ErrorContains(t, err, "HTTP 302")
	assert.False(t, snapshot.Enabled("released"), "redirect responses cannot supply global flags")
	assert.Nil(t, client.CheckRedirect, "the shared client must not be reconfigured")
}

func TestLoadAcceptsResponseAtOneMiBSizeLimit(t *testing.T) {
	t.Parallel()

	// Given a valid response exactly at the supported size limit
	body := `{"flags":{"released":true}}`
	body += strings.Repeat(" ", (1<<20)-len(body))
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.MatchAny, httpmock.StringResponse(body))

	// When the global snapshot is loaded
	snapshot, err := Load(t.Context(), "https://flags.example", t.TempDir(), &http.Client{Transport: reg})

	// Then the inclusive size boundary permits its flag values
	require.NoError(t, err)
	assert.True(t, snapshot.Enabled("released"), "a complete response at the limit is still valid")
}
