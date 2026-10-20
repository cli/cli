package globalflags

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const maxCacheSize = 2 << 20

type cacheEntry struct {
	Endpoint  string          `json:"endpoint"`
	FetchedAt time.Time       `json:"fetched_at"`
	Response  json.RawMessage `json:"response"`
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

func writeCache(path, endpoint string, response []byte) error {
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
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create global flag cache directory: %w", err)
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".global-flags-*")
	if err != nil {
		return fmt.Errorf("create global flag cache: %w", err)
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		file.Close()
		return fmt.Errorf("write global flag cache: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close global flag cache: %w", err)
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return fmt.Errorf("replace global flag cache: %w", err)
	}
	return nil
}
