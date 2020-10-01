package api

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func makeCachedClient(httpClient *http.Client, cacheTTL time.Duration) *http.Client {
	cacheDir := filepath.Join(os.TempDir(), "gh-cli-cache")
	return &http.Client{
		Transport: CacheReponse(cacheTTL, cacheDir)(httpClient.Transport),
	}
}

// CacheReponse produces a RoundTripper that caches HTTP responses to disk for a specified amount of time
func CacheReponse(ttl time.Duration, dir string) ClientOption {
	return func(tr http.RoundTripper) http.RoundTripper {
		return &funcTripper{roundTrip: func(req *http.Request) (*http.Response, error) {
			key, keyErr := cacheKey(req)
			cacheFile := filepath.Join(dir, key)
			if keyErr == nil {
				// TODO: make thread-safe
				if res, err := readCache(ttl, cacheFile, req); err == nil {
					return res, nil
				}
			}
			res, err := tr.RoundTrip(req)
			if err == nil && keyErr == nil {
				// TODO: make thread-safe
				_ = writeCache(cacheFile, res)
			}
			return res, err
		}}
	}
}

func cacheKey(req *http.Request) (string, error) {
	h := sha256.New()
	fmt.Fprintf(h, "%s:", req.Method)
	fmt.Fprintf(h, "%s:", req.URL.String())

	if req.Body != nil {
		bodyCopy := &bytes.Buffer{}
		defer req.Body.Close()
		_, err := io.Copy(h, io.TeeReader(req.Body, bodyCopy))
		req.Body = ioutil.NopCloser(bodyCopy)
		if err != nil {
			return "", err
		}
	}

	digest := h.Sum(nil)
	return fmt.Sprintf("%x", digest), nil
}

type readCloser struct {
	io.Reader
	io.Closer
}

func readCache(ttl time.Duration, cacheFile string, req *http.Request) (*http.Response, error) {
	f, err := os.Open(cacheFile)
	if err != nil {
		return nil, err
	}

	fs, err := f.Stat()
	if err != nil {
		return nil, err
	}

	age := time.Since(fs.ModTime())
	if age > ttl {
		return nil, errors.New("cache expired")
	}

	res, err := http.ReadResponse(bufio.NewReader(f), req)
	if res == nil {
		res.Body = &readCloser{
			Reader: res.Body,
			Closer: f,
		}
	} else {
		f.Close()
	}
	return res, err
}

func writeCache(cacheFile string, res *http.Response) error {
	err := os.MkdirAll(filepath.Dir(cacheFile), 0755)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(cacheFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	bodyCopy := &bytes.Buffer{}
	defer res.Body.Close()
	res.Body = ioutil.NopCloser(io.TeeReader(res.Body, bodyCopy))
	err = res.Write(f)
	res.Body = ioutil.NopCloser(bodyCopy)
	return err
}
