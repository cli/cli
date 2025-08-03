package ratelimits

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ErrBucketNotFound indicates that the bucket was not found.
var ErrBucketNotFound = fmt.Errorf("bucket not found")

// Source is an interface for creating and modifying TATs.
type Source interface {
	// BatchSet stores the TATs at the specified bucketKeys (formatted as
	// 'name:id'). Implementations MUST ensure non-blocking operations by
	// either:
	//   a) applying a deadline or timeout to the context WITHIN the method, or
	//   b) guaranteeing the operation will not block indefinitely (e.g. via
	//    the underlying storage client implementation).
	BatchSet(ctx context.Context, bucketKeys map[string]time.Time) error

	// BatchSetNotExisting attempts to set TATs for the specified bucketKeys if
	// they do not already exist. Returns a map indicating which keys already
	// exist.
	BatchSetNotExisting(ctx context.Context, buckets map[string]time.Time) (map[string]bool, error)

	// BatchIncrement updates the TATs for the specified bucketKeys, similar to
	// BatchSet. Implementations MUST ensure non-blocking operations by either:
	//   a) applying a deadline or timeout to the context WITHIN the method, or
	//   b) guaranteeing the operation will not block indefinitely (e.g. via
	//    the underlying storage client implementation).
	BatchIncrement(ctx context.Context, buckets map[string]increment) error

	// Get retrieves the TAT associated with the specified bucketKey (formatted
	// as 'name:id'). Implementations MUST ensure non-blocking operations by
	// either:
	//   a) applying a deadline or timeout to the context WITHIN the method, or
	//   b) guaranteeing the operation will not block indefinitely (e.g. via
	//    the underlying storage client implementation).
	Get(ctx context.Context, bucketKey string) (time.Time, error)

	// BatchGet retrieves the TATs associated with the specified bucketKeys
	// (formatted as 'name:id'). Implementations MUST ensure non-blocking
	// operations by either:
	//   a) applying a deadline or timeout to the context WITHIN the method, or
	//   b) guaranteeing the operation will not block indefinitely (e.g. via
	//    the underlying storage client implementation).
	BatchGet(ctx context.Context, bucketKeys []string) (map[string]time.Time, error)

	// Delete removes the TAT associated with the specified bucketKey (formatted
	// as 'name:id'). Implementations MUST ensure non-blocking operations by
	// either:
	//   a) applying a deadline or timeout to the context WITHIN the method, or
	//   b) guaranteeing the operation will not block indefinitely (e.g. via
	//    the underlying storage client implementation).
	Delete(ctx context.Context, bucketKey string) error
}

type increment struct {
	cost time.Duration
	ttl  time.Duration
}

// inmem is an in-memory implementation of the source interface used for
// testing.
type inmem struct {
	sync.RWMutex
	m map[string]time.Time
}

var _ Source = (*inmem)(nil)

func NewInmemSource() *inmem {
	return &inmem{m: make(map[string]time.Time)}
}

func (in *inmem) BatchSet(_ context.Context, bucketKeys map[string]time.Time) error {
	in.Lock()
	defer in.Unlock()
	for k, v := range bucketKeys {
		in.m[k] = v
	}
	return nil
}

func (in *inmem) BatchSetNotExisting(_ context.Context, bucketKeys map[string]time.Time) (map[string]bool, error) {
	in.Lock()
	defer in.Unlock()
	alreadyExists := make(map[string]bool, len(bucketKeys))
	for k, v := range bucketKeys {
		_, ok := in.m[k]
		if ok {
			alreadyExists[k] = true
		} else {
			in.m[k] = v
		}
	}
	return alreadyExists, nil
}

func (in *inmem) BatchIncrement(_ context.Context, bucketKeys map[string]increment) error {
	in.Lock()
	defer in.Unlock()
	for k, v := range bucketKeys {
		in.m[k] = in.m[k].Add(v.cost)
	}
	return nil
}

func (in *inmem) Get(_ context.Context, bucketKey string) (time.Time, error) {
	in.RLock()
	defer in.RUnlock()
	tat, ok := in.m[bucketKey]
	if !ok {
		return time.Time{}, ErrBucketNotFound
	}
	return tat, nil
}

func (in *inmem) BatchGet(_ context.Context, bucketKeys []string) (map[string]time.Time, error) {
	in.RLock()
	defer in.RUnlock()
	tats := make(map[string]time.Time, len(bucketKeys))
	for _, k := range bucketKeys {
		tat, ok := in.m[k]
		if !ok {
			continue
		}
		tats[k] = tat
	}
	return tats, nil
}

func (in *inmem) Delete(_ context.Context, bucketKey string) error {
	in.Lock()
	defer in.Unlock()
	delete(in.m, bucketKey)
	return nil
}
