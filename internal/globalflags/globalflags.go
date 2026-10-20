// Package globalflags provides an immutable, unauthenticated global flag snapshot.
// Its development-only wire contract is provisional, not an existing CAFE API.
package globalflags

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

const (
	rpcPath         = "/twirp/github.cli.globalflags.v1.GlobalFeatureFlags/GetFlags"
	maxResponseSize = 1 << 20
)

// Snapshot contains global flag values fixed for the lifetime of an invocation.
// The zero value disables every flag.
type Snapshot struct {
	values map[string]bool
}

// NewSnapshot copies flag values into a snapshot without performing any I/O.
func NewSnapshot(values map[string]bool) Snapshot {
	return Snapshot{values: maps.Clone(values)}
}

// Enabled reports whether a flag is enabled. Unknown flags are disabled.
func (s Snapshot) Enabled(name string) bool {
	return s.values[name]
}

// Load obtains a snapshot from the provisional global Twirp service, reusing a
// cache for thirty minutes. An empty serverURL disables lookup and cache access.
// Refreshes have a two-second deadline. On failure, the last valid snapshot (or
// defaults) is returned alongside the error. Cache failures do not prevent
// obtaining server flags. The HTTP client must not attach credentials.
func Load(ctx context.Context, serverURL, cacheDir string, client *http.Client) (Snapshot, error) {
	if serverURL == "" {
		return Snapshot{}, nil
	}
	baseURL, err := url.Parse(serverURL)
	if err != nil || (baseURL.Scheme != "http" && baseURL.Scheme != "https") ||
		baseURL.Hostname() == "" || baseURL.User != nil || baseURL.RawQuery != "" ||
		baseURL.ForceQuery || baseURL.Fragment != "" {
		return Snapshot{}, errors.New("global flag server URL must be an absolute HTTP(S) URL without credentials, query, or fragment")
	}
	endpoint := strings.TrimRight(baseURL.String(), "/") + rpcPath
	cachePath := filepath.Join(cacheDir, "global-feature-flags", "development-v1.json")
	cached, fetchedAt, cacheErr := readCache(cachePath, endpoint)
	if cacheErr == nil && !fetchedAt.IsZero() && time.Since(fetchedAt) < 30*time.Minute {
		return cached, nil
	}

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	snapshot, body, err := fetch(ctx, endpoint, client)
	if err != nil {
		return cached, errors.Join(cacheErr, err)
	}
	return snapshot, errors.Join(cacheErr, writeCache(cachePath, endpoint, body))
}

func fetch(ctx context.Context, endpoint string, client *http.Client) (Snapshot, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader("{}"))
	if err != nil {
		return Snapshot{}, nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	noRedirects := *client
	noRedirects.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := noRedirects.Do(req)
	if err != nil {
		return Snapshot{}, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Snapshot{}, nil, fmt.Errorf("global flags: HTTP %d", resp.StatusCode)
	}
	body, err := readLimited(resp.Body, maxResponseSize)
	if err != nil {
		return Snapshot{}, nil, fmt.Errorf("read global flag response: %w", err)
	}
	snapshot, err := decodeFlags(body)
	return snapshot, body, err
}

func decodeFlags(body []byte) (Snapshot, error) {
	var response struct {
		Flags map[string]*bool `json:"flags"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return Snapshot{}, fmt.Errorf("decode global flag response: %w", err)
	}
	if response.Flags == nil {
		return Snapshot{}, errors.New("global flag response must contain a flags object")
	}
	values := make(map[string]bool, len(response.Flags))
	for name, value := range response.Flags {
		if strings.TrimSpace(name) == "" || value == nil {
			return Snapshot{}, errors.New("global flags must have nonblank names and boolean values")
		}
		values[name] = *value
	}
	return Snapshot{values: values}, nil
}

func readLimited(r io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("exceeds %d byte limit", limit)
	}
	return data, nil
}
