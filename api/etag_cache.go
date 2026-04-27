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
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("creating etag cache dir: %w", err)
	}
	return &etagCache{dir: dir}, nil
}

// cacheKey computes a SHA-256 hash of the URL and request headers that affect
// authorization and response representation.
func cacheKey(req *http.Request) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\n%s\n%s\n%s",
		req.URL.String(),
		req.Header.Get("Authorization"),
		req.Header.Get("Accept"),
		req.Header.Get("X-GitHub-Api-Version"),
	)
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

// Store serializes a full HTTP response to disk atomically.
func (c *etagCache) Store(key string, resp *http.Response) (err error) {
	tmp, err := os.CreateTemp(c.dir, key+".tmp-*")
	if err != nil {
		return err
	}

	tmpPath := tmp.Name()
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmpPath)
		}
	}()

	if err = resp.Write(tmp); err != nil {
		return err
	}

	if err = tmp.Close(); err != nil {
		return err
	}

	return os.Rename(tmpPath, c.path(key))
}
