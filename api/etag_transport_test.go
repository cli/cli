package api

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEtagTransport_FirstRequest(t *testing.T) {
	cache := newTestEtagCache(t)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.Header.Get("If-None-Match"))
		w.Header().Set("ETag", `W/"first"`)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"name":"cli"}`))
	}))
	defer ts.Close()

	transport := &etagTransport{
		transport: http.DefaultTransport,
		cache:     cache,
	}
	client := &http.Client{Transport: transport}

	resp, err := client.Get(ts.URL + "/repos/cli/cli")
	require.NoError(t, err)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, `{"name":"cli"}`, string(body))

	// Verify response was cached (written on Close)
	req, _ := http.NewRequest("GET", ts.URL+"/repos/cli/cli", nil)
	key := cacheKey(req)
	cached := cache.Load(key)
	require.NotNil(t, cached)
	defer cached.Body.Close()
	assert.Equal(t, `W/"first"`, cached.Header.Get("ETag"))
	assert.Equal(t, "application/json", cached.Header.Get("Content-Type"))
}

func TestEtagTransport_ConditionalHit(t *testing.T) {
	cache := newTestEtagCache(t)

	var gotIfNoneMatch string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIfNoneMatch = r.Header.Get("If-None-Match")
		w.WriteHeader(http.StatusNotModified)
	}))
	defer ts.Close()

	// Pre-populate cache
	req, _ := http.NewRequest("GET", ts.URL+"/repos/cli/cli", nil)
	key := cacheKey(req)
	storeTestResponse(t, cache, key, map[string]string{
		"ETag":         `W/"cached"`,
		"Content-Type": "application/json",
	}, `{"cached":true}`)

	transport := &etagTransport{
		transport: http.DefaultTransport,
		cache:     cache,
	}
	client := &http.Client{Transport: transport}

	resp, err := client.Get(ts.URL + "/repos/cli/cli")
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, `W/"cached"`, gotIfNoneMatch)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, `{"cached":true}`, string(body))
	assert.Equal(t, "HIT", resp.Header.Get("X-GH-ETag-Cache"))
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
}

func TestEtagTransport_ConditionalMiss(t *testing.T) {
	cache := newTestEtagCache(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `W/"new"`)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"updated":true}`))
	}))
	defer ts.Close()

	// Pre-populate cache with old data
	req, _ := http.NewRequest("GET", ts.URL+"/repos/cli/cli", nil)
	key := cacheKey(req)
	storeTestResponse(t, cache, key, map[string]string{
		"ETag":         `W/"old"`,
		"Content-Type": "application/json",
	}, `{"old":true}`)

	transport := &etagTransport{
		transport: http.DefaultTransport,
		cache:     cache,
	}
	client := &http.Client{Transport: transport}

	resp, err := client.Get(ts.URL + "/repos/cli/cli")
	require.NoError(t, err)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, `{"updated":true}`, string(body))

	// Verify cache was updated (written on Close)
	cached := cache.Load(key)
	require.NotNil(t, cached)
	defer cached.Body.Close()
	assert.Equal(t, `W/"new"`, cached.Header.Get("ETag"))
}

func TestEtagTransport_NonGET_Bypassed(t *testing.T) {
	cache := newTestEtagCache(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.Header.Get("If-None-Match"))
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"created":true}`))
	}))
	defer ts.Close()

	transport := &etagTransport{
		transport: http.DefaultTransport,
		cache:     cache,
	}
	client := &http.Client{Transport: transport}

	resp, err := client.Post(ts.URL+"/repos/cli/cli/issues", "application/json", bytes.NewBufferString(`{}`))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)
}

func TestEtagTransport_NoETagHeader(t *testing.T) {
	cache := newTestEtagCache(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"no_etag":true}`))
	}))
	defer ts.Close()

	transport := &etagTransport{
		transport: http.DefaultTransport,
		cache:     cache,
	}
	client := &http.Client{Transport: transport}

	resp, err := client.Get(ts.URL + "/repos/cli/cli")
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, `{"no_etag":true}`, string(body))

	// Verify nothing was cached
	req, _ := http.NewRequest("GET", ts.URL+"/repos/cli/cli", nil)
	key := cacheKey(req)
	assert.Nil(t, cache.Load(key))
}

func TestEtagTransport_CacheCorruption(t *testing.T) {
	cache := newTestEtagCache(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotModified)
	}))
	defer ts.Close()

	// Write garbage to the cache file
	req, _ := http.NewRequest("GET", ts.URL+"/repos/cli/cli", nil)
	key := cacheKey(req)
	require.NoError(t, os.WriteFile(cache.path(key), []byte("not a valid response"), 0644))

	transport := &etagTransport{
		transport: http.DefaultTransport,
		cache:     cache,
	}
	client := &http.Client{Transport: transport}

	resp, err := client.Get(ts.URL + "/repos/cli/cli")
	require.NoError(t, err)
	defer resp.Body.Close()

	// Corrupt cache can't be parsed, so no If-None-Match is sent,
	// but server still returned 304. No cached response to fall back to,
	// so we get the 304 as-is.
	assert.Equal(t, http.StatusNotModified, resp.StatusCode)
}
