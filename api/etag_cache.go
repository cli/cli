package api

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	ghConfig "github.com/cli/go-gh/v2/pkg/config"
)

// etagCache manages on-disk storage of full HTTP responses keyed by request.
type etagCache struct {
	dir string
}

func newEtagCache() (*etagCache, error) {
	dir := filepath.Join(ghConfig.CacheDir(), "etag")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("creating etag cache dir: %w", err)
	}
	return &etagCache{dir: dir}, nil
}

// cacheKey computes a SHA-256 hash of the URL and Authorization header.
func cacheKey(req *http.Request) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\n%s", req.URL.String(), req.Header.Get("Authorization"))
	return hex.EncodeToString(h.Sum(nil))
}

func (c *etagCache) path(key string) string {
	return filepath.Join(c.dir, key)
}

// Load reads a cached HTTP response from disk, or returns nil if not cached.
func (c *etagCache) Load(key string) *http.Response {
	data, err := os.ReadFile(c.path(key))
	if err != nil {
		return nil
	}
	resp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(data)), nil)
	if err != nil {
		return nil
	}
	return resp
}

// Store serializes a full HTTP response to disk.
func (c *etagCache) Store(key string, resp *http.Response) error {
	f, err := os.Create(c.path(key))
	if err != nil {
		return err
	}
	defer f.Close()
	return resp.Write(f)
}
