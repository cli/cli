package api

import (
	"bytes"
	"io"
	"net/http"
)

// etagTransport is an http.RoundTripper that implements ETag-based conditional requests.
// On 304 Not Modified, it returns the full cached response.
type etagTransport struct {
	transport http.RoundTripper
	cache     *etagCache
}

func (t *etagTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Only cache GET requests
	if req.Method != http.MethodGet {
		return t.transport.RoundTrip(req)
	}

	key := cacheKey(req)

	// Load cached response to get the ETag for conditional request
	cachedResp := t.cache.Load(key)
	if cachedResp != nil {
		if etag := cachedResp.Header.Get("ETag"); etag != "" {
			req.Header.Set("If-None-Match", etag)
		}
	}

	resp, err := t.transport.RoundTrip(req)
	if err != nil {
		if cachedResp != nil {
			cachedResp.Body.Close()
		}
		return resp, err
	}

	if resp.StatusCode == http.StatusNotModified && cachedResp != nil {
		resp.Body.Close()
		cachedResp.Header.Set("X-GH-ETag-Cache", "HIT")
		return cachedResp, nil
	}

	// Done with cached response if we're not using it
	if cachedResp != nil {
		cachedResp.Body.Close()
	}

	if resp.StatusCode == http.StatusOK && resp.Header.Get("ETag") != "" {
		resp.Body = &etagCacheWriter{
			ReadCloser: resp.Body,
			cache:      t.cache,
			key:        key,
			header:     resp.Header.Clone(),
			buf:        &bytes.Buffer{},
		}
	}

	return resp, nil
}

// etagCacheWriter wraps a response body, buffering reads so the full response
// can be written to the cache when the body is closed.
type etagCacheWriter struct {
	io.ReadCloser
	cache   *etagCache
	key     string
	header  http.Header
	buf     *bytes.Buffer
	sawEOF  bool
	readErr bool
}

func (w *etagCacheWriter) Read(p []byte) (int, error) {
	n, err := w.ReadCloser.Read(p)
	if n > 0 {
		w.buf.Write(p[:n])
	}
	if err == io.EOF {
		w.sawEOF = true
	} else if err != nil {
		w.readErr = true
	}
	return n, err
}

func (w *etagCacheWriter) Close() error {
	err := w.ReadCloser.Close()
	if !w.sawEOF || w.readErr {
		return err
	}
	// Reconstruct a full response for caching
	cachedResp := &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     w.header,
		Body:       io.NopCloser(bytes.NewReader(w.buf.Bytes())),
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
	}
	// Best-effort cache write
	_ = w.cache.Store(w.key, cachedResp)
	return err
}
