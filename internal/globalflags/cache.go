package globalflags

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cli/cli/v2/internal/flock"
)

const maxCacheSize = 2 << 20
const maxRefreshStateSize = 4 << 10

const refreshRetryInterval = time.Minute

type cacheEntry struct {
	Endpoint  string          `json:"endpoint"`
	FetchedAt time.Time       `json:"fetched_at"`
	Response  json.RawMessage `json:"response"`
}

type refreshState struct {
	Endpoint    string    `json:"endpoint"`
	AttemptedAt time.Time `json:"attempted_at"`
	LastError   string    `json:"last_error,omitempty"`
}

// RefreshRequest records ownership of the current refresh attempt.
type RefreshRequest struct {
	path        string
	endpoint    string
	attemptedAt time.Time
}

// BeginRefresh records a refresh attempt unless the same endpoint was attempted
// recently. Invalid endpoint configuration is rejected before a worker starts.
func BeginRefresh(serverURL, cacheDir string) (*RefreshRequest, error) {
	endpoint, err := endpointURL(serverURL)
	if err != nil {
		return nil, err
	}
	statePath := cachePath(cacheDir) + ".refresh"
	if err := os.MkdirAll(filepath.Dir(statePath), 0o700); err != nil {
		return nil, fmt.Errorf("create global flag cache directory: %w", err)
	}
	_, unlock, err := flock.TryLock(statePath + ".lock")
	if errors.Is(err, flock.ErrLocked) {
		return nil, nil
	}
	if err != nil && !flock.IsUnsupported(err) {
		return nil, fmt.Errorf("lock global flag refresh request: %w", err)
	}
	if err == nil {
		defer unlock()
	}

	state, stateErr := readRefreshState(statePath)
	if errors.Is(stateErr, os.ErrNotExist) {
		stateErr = nil
	}
	if stateErr == nil && state.Endpoint == endpoint &&
		!state.AttemptedAt.IsZero() && time.Since(state.AttemptedAt) < refreshRetryInterval {
		if state.LastError != "" {
			return nil, fmt.Errorf("last global flag refresh failed: %s", state.LastError)
		}
		return nil, nil
	}
	attemptedAt := time.Now()
	if err := writeRefreshState(statePath, refreshState{Endpoint: endpoint, AttemptedAt: attemptedAt}); err != nil {
		return nil, err
	}
	return &RefreshRequest{
		path:        statePath,
		endpoint:    endpoint,
		attemptedAt: attemptedAt,
	}, stateErr
}

// RecordRefreshFailure preserves a detached worker error for the next debug
// invocation while retaining the retry cooldown.
func RecordRefreshFailure(serverURL, cacheDir string, refreshErr error) error {
	if refreshErr == nil {
		return errors.New("global flag refresh failure is nil")
	}
	endpoint, err := endpointURL(serverURL)
	if err != nil {
		return err
	}
	statePath := cachePath(cacheDir) + ".refresh"
	if err := os.MkdirAll(filepath.Dir(statePath), 0o700); err != nil {
		return fmt.Errorf("create global flag cache directory: %w", err)
	}
	_, unlock, err := flock.TryLock(statePath + ".lock")
	if errors.Is(err, flock.ErrLocked) {
		return nil
	}
	if err != nil && !flock.IsUnsupported(err) {
		return fmt.Errorf("lock global flag refresh request: %w", err)
	}
	if err == nil {
		defer unlock()
	}
	state, stateErr := readRefreshState(statePath)
	if stateErr != nil || state.Endpoint != endpoint {
		state = refreshState{Endpoint: endpoint, AttemptedAt: time.Now()}
	}
	state.LastError = refreshErr.Error()
	if len(state.LastError) > 1024 {
		state.LastError = state.LastError[:1024]
	}
	return writeRefreshState(statePath, state)
}

// Cancel releases a refresh attempt whose worker failed to start.
func (r *RefreshRequest) Cancel() error {
	_, unlock, err := flock.TryLock(r.path + ".lock")
	if errors.Is(err, flock.ErrLocked) {
		return nil
	}
	if err != nil && !flock.IsUnsupported(err) {
		return fmt.Errorf("lock global flag refresh request: %w", err)
	}
	if err == nil {
		defer unlock()
	}
	state, err := readRefreshState(r.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if state.Endpoint != r.endpoint || !state.AttemptedAt.Equal(r.attemptedAt) {
		return nil
	}
	if err := os.Remove(r.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove global flag refresh state: %w", err)
	}
	return nil
}

func writeRefreshState(path string, state refreshState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode global flag refresh state: %w", err)
	}
	if err := writeAtomic(path, data); err != nil {
		return fmt.Errorf("write global flag refresh state: %w", err)
	}
	return nil
}

func readRefreshState(path string) (refreshState, error) {
	file, err := os.Open(path)
	if err != nil {
		return refreshState{}, err
	}
	defer file.Close()
	data, err := readLimited(file, maxRefreshStateSize)
	if err != nil {
		return refreshState{}, fmt.Errorf("read global flag refresh state: %w", err)
	}
	var state refreshState
	if err := json.Unmarshal(data, &state); err != nil {
		return refreshState{}, fmt.Errorf("decode global flag refresh state: %w", err)
	}
	if state.Endpoint == "" || state.AttemptedAt.IsZero() || state.AttemptedAt.After(time.Now()) {
		return refreshState{}, errors.New("global flag refresh state is invalid")
	}
	return state, nil
}

func readCache(path, endpoint string) (Snapshot, time.Time, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return Snapshot{}, time.Time{}, nil
	}
	if err != nil {
		return Snapshot{}, time.Time{}, fmt.Errorf("read global flag cache: %w", err)
	}
	defer file.Close()
	data, err := readLimited(file, maxCacheSize)
	if err != nil {
		return Snapshot{}, time.Time{}, fmt.Errorf("read global flag cache: %w", err)
	}
	var entry cacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return Snapshot{}, time.Time{}, fmt.Errorf("decode global flag cache: %w", err)
	}
	if entry.Endpoint == "" {
		return Snapshot{}, time.Time{}, errors.New("global flag cache has no endpoint")
	}
	if entry.Endpoint != endpoint {
		return Snapshot{}, time.Time{}, nil
	}
	if entry.FetchedAt.IsZero() || entry.FetchedAt.After(time.Now()) {
		return Snapshot{}, time.Time{}, errors.New("global flag cache has an invalid fetch time")
	}
	snapshot, err := decodeFlags(entry.Response)
	if err != nil {
		return Snapshot{}, time.Time{}, fmt.Errorf("decode cached global flags: %w", err)
	}
	return snapshot, entry.FetchedAt, nil
}

func writeCache(path, endpoint string, values map[string]bool) error {
	response, err := json.Marshal(struct {
		Flags map[string]bool `json:"flags"`
	}{Flags: values})
	if err != nil {
		return fmt.Errorf("encode global flag response: %w", err)
	}
	data, err := json.Marshal(cacheEntry{
		Endpoint:  endpoint,
		FetchedAt: time.Now(),
		Response:  response,
	})
	if err != nil {
		return fmt.Errorf("encode global flag cache: %w", err)
	}
	if len(data) > maxCacheSize {
		return fmt.Errorf("global flag cache exceeds %d byte limit", maxCacheSize)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create global flag cache directory: %w", err)
	}
	return writeAtomic(path, data)
}

func writeAtomic(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".global-flags-*")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		file.Close()
		return fmt.Errorf("write temporary file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close temporary file: %w", err)
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return fmt.Errorf("replace file: %w", err)
	}
	return nil
}
