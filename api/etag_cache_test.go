package api

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestEtagCache(t *testing.T) *etagCache {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "etag")
	require.NoError(t, os.MkdirAll(dir, 0755))
	return &etagCache{dir: dir}
}

func storeTestResponse(t *testing.T, cache *etagCache, key string, headers map[string]string, body string) {
	t.Helper()
	h := make(http.Header)
	for k, v := range headers {
		h.Set(k, v)
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     h,
		Body:       io.NopCloser(bytes.NewBufferString(body)),
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
	}
	require.NoError(t, cache.Store(key, resp))
}

func TestEtagCache_StoreAndLoad(t *testing.T) {
	cache := newTestEtagCache(t)
	key := "testkey"

	storeTestResponse(t, cache, key, map[string]string{
		"ETag":         `W/"abc123"`,
		"Content-Type": "application/json",
	}, `{"id":1}`)

	resp := cache.Load(key)
	require.NotNil(t, resp)
	defer resp.Body.Close()

	assert.Equal(t, `W/"abc123"`, resp.Header.Get("ETag"))
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, `{"id":1}`, string(b))
}

func TestEtagCache_Load_NotCached(t *testing.T) {
	cache := newTestEtagCache(t)
	assert.Nil(t, cache.Load("nonexistent"))
}

func TestEtagCache_CacheKey(t *testing.T) {
	req1, _ := http.NewRequest("GET", "https://api.github.com/repos/cli/cli", nil)
	req1.Header.Set("Authorization", "token AAA")

	req2, _ := http.NewRequest("GET", "https://api.github.com/repos/cli/cli", nil)
	req2.Header.Set("Authorization", "token BBB")

	req3, _ := http.NewRequest("GET", "https://api.github.com/repos/cli/cli", nil)
	req3.Header.Set("Authorization", "token AAA")

	key1 := cacheKey(req1)
	key2 := cacheKey(req2)
	key3 := cacheKey(req3)

	assert.NotEqual(t, key1, key2, "different tokens should produce different keys")
	assert.Equal(t, key1, key3, "same URL and token should produce the same key")
}
