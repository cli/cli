package wfe2

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jmhodges/clock"
	corepb "github.com/letsencrypt/boulder/core/proto"
	"github.com/letsencrypt/boulder/metrics"
	sapb "github.com/letsencrypt/boulder/sa/proto"
	"github.com/letsencrypt/boulder/test"
	"google.golang.org/grpc"
)

type recordingBackend struct {
	requests []int64
}

func (rb *recordingBackend) GetRegistration(
	ctx context.Context,
	regID *sapb.RegistrationID,
	opts ...grpc.CallOption,
) (*corepb.Registration, error) {
	rb.requests = append(rb.requests, regID.Id)
	return &corepb.Registration{
		Id:      regID.Id,
		Contact: []string{"example@example.com"},
	}, nil
}

func TestCacheAddRetrieve(t *testing.T) {
	ctx := context.Background()
	backend := &recordingBackend{}

	cache := NewAccountCache(backend, 10, time.Second, clock.NewFake(), metrics.NoopRegisterer)

	result, err := cache.GetRegistration(ctx, &sapb.RegistrationID{Id: 1234})
	test.AssertNotError(t, err, "getting registration")
	test.AssertEquals(t, result.Id, int64(1234))
	test.AssertEquals(t, len(backend.requests), 1)

	// Request it again. This should hit the cache so our backend should not see additional requests.
	result, err = cache.GetRegistration(ctx, &sapb.RegistrationID{Id: 1234})
	test.AssertNotError(t, err, "getting registration")
	test.AssertEquals(t, result.Id, int64(1234))
	test.AssertEquals(t, len(backend.requests), 1)
}

// Test that the cache copies values before giving them out, so code that receives a cached
// value can't modify the cache's contents.
func TestCacheCopy(t *testing.T) {
	ctx := context.Background()
	backend := &recordingBackend{}

	cache := NewAccountCache(backend, 10, time.Second, clock.NewFake(), metrics.NoopRegisterer)

	_, err := cache.GetRegistration(ctx, &sapb.RegistrationID{Id: 1234})
	test.AssertNotError(t, err, "getting registration")
	test.AssertEquals(t, len(backend.requests), 1)

	test.AssertEquals(t, cache.cache.Len(), 1)

	// Request it again. This should hit the cache.
	result, err := cache.GetRegistration(ctx, &sapb.RegistrationID{Id: 1234})
	test.AssertNotError(t, err, "getting registration")
	test.AssertEquals(t, len(backend.requests), 1)

	// Modify a pointer value inside the result
	result.Contact[0] = "different@example.com"

	result, err = cache.GetRegistration(ctx, &sapb.RegistrationID{Id: 1234})
	test.AssertNotError(t, err, "getting registration")
	test.AssertEquals(t, len(backend.requests), 1)

	test.AssertDeepEquals(t, result.Contact, []string{"example@example.com"})
}

// Test that the cache expires values.
func TestCacheExpires(t *testing.T) {
	ctx := context.Background()
	backend := &recordingBackend{}

	clk := clock.NewFake()
	cache := NewAccountCache(backend, 10, time.Second, clk, metrics.NoopRegisterer)

	_, err := cache.GetRegistration(ctx, &sapb.RegistrationID{Id: 1234})
	test.AssertNotError(t, err, "getting registration")
	test.AssertEquals(t, len(backend.requests), 1)

	// Request it again. This should hit the cache.
	_, err = cache.GetRegistration(ctx, &sapb.RegistrationID{Id: 1234})
	test.AssertNotError(t, err, "getting registration")
	test.AssertEquals(t, len(backend.requests), 1)

	test.AssertEquals(t, cache.cache.Len(), 1)

	// "Sleep" 10 seconds to expire the entry
	clk.Sleep(10 * time.Second)

	// This should not hit the cache
	_, err = cache.GetRegistration(ctx, &sapb.RegistrationID{Id: 1234})
	test.AssertNotError(t, err, "getting registration")
	test.AssertEquals(t, len(backend.requests), 2)
}

type wrongIDBackend struct{}

func (wib wrongIDBackend) GetRegistration(
	ctx context.Context,
	regID *sapb.RegistrationID,
	opts ...grpc.CallOption,
) (*corepb.Registration, error) {
	return &corepb.Registration{
		Id:      regID.Id + 1,
		Contact: []string{"example@example.com"},
	}, nil
}

func TestWrongId(t *testing.T) {
	ctx := context.Background()
	cache := NewAccountCache(wrongIDBackend{}, 10, time.Second, clock.NewFake(), metrics.NoopRegisterer)

	_, err := cache.GetRegistration(ctx, &sapb.RegistrationID{Id: 1234})
	test.AssertError(t, err, "expected error when backend returns wrong ID")
}

type errorBackend struct{}

func (eb errorBackend) GetRegistration(ctx context.Context,
	regID *sapb.RegistrationID,
	opts ...grpc.CallOption,
) (*corepb.Registration, error) {
	return nil, errors.New("some error")
}

func TestErrorPassthrough(t *testing.T) {
	ctx := context.Background()
	cache := NewAccountCache(errorBackend{}, 10, time.Second, clock.NewFake(), metrics.NoopRegisterer)

	_, err := cache.GetRegistration(ctx, &sapb.RegistrationID{Id: 1234})
	test.AssertError(t, err, "expected error when backend errors")
	test.AssertEquals(t, err.Error(), "some error")
}
