package email

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	emailpb "github.com/letsencrypt/boulder/email/proto"
	blog "github.com/letsencrypt/boulder/log"
	"github.com/letsencrypt/boulder/metrics"
	"github.com/letsencrypt/boulder/test"

	"github.com/prometheus/client_golang/prometheus"
)

var ctx = context.Background()

// mockPardotClientImpl is a mock implementation of PardotClient.
type mockPardotClientImpl struct {
	sync.Mutex
	CreatedContacts []string
}

// newMockPardotClientImpl returns a MockPardotClientImpl, implementing the
// PardotClient interface. Both refer to the same instance, with the interface
// for mock interaction and the struct for state inspection and modification.
func newMockPardotClientImpl() (PardotClient, *mockPardotClientImpl) {
	mockImpl := &mockPardotClientImpl{
		CreatedContacts: []string{},
	}
	return mockImpl, mockImpl
}

// SendContact adds an email to CreatedContacts.
func (m *mockPardotClientImpl) SendContact(email string) error {
	m.Lock()
	m.CreatedContacts = append(m.CreatedContacts, email)
	m.Unlock()
	return nil
}

func (m *mockPardotClientImpl) getCreatedContacts() []string {
	m.Lock()
	defer m.Unlock()

	// Return a copy to avoid race conditions.
	return slices.Clone(m.CreatedContacts)
}

// setup creates a new ExporterImpl, a MockPardotClientImpl, and the start and
// cleanup functions for the ExporterImpl. Call start() to begin processing the
// ExporterImpl queue and cleanup() to drain and shutdown. If start() is called,
// cleanup() must be called.
func setup() (*ExporterImpl, *mockPardotClientImpl, func(), func()) {
	mockClient, clientImpl := newMockPardotClientImpl()
	exporter := NewExporterImpl(mockClient, nil, 1000000, 5, metrics.NoopRegisterer, blog.NewMock())
	daemonCtx, cancel := context.WithCancel(context.Background())
	return exporter, clientImpl,
		func() { exporter.Start(daemonCtx) },
		func() {
			cancel()
			exporter.Drain()
		}
}

func TestSendContacts(t *testing.T) {
	t.Parallel()

	exporter, clientImpl, start, cleanup := setup()
	start()
	defer cleanup()

	wantContacts := []string{"test@example.com", "user@example.com"}
	_, err := exporter.SendContacts(ctx, &emailpb.SendContactsRequest{
		Emails: wantContacts,
	})
	test.AssertNotError(t, err, "Error creating contacts")

	var gotContacts []string
	for range 100 {
		gotContacts = clientImpl.getCreatedContacts()
		if len(gotContacts) == 2 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	test.AssertSliceContains(t, gotContacts, wantContacts[0])
	test.AssertSliceContains(t, gotContacts, wantContacts[1])

	// Check that the error counter was not incremented.
	test.AssertMetricWithLabelsEquals(t, exporter.pardotErrorCounter, prometheus.Labels{}, 0)
}

func TestSendContactsQueueFull(t *testing.T) {
	t.Parallel()

	exporter, _, start, cleanup := setup()
	start()
	defer cleanup()

	var err error
	for range contactsQueueCap * 2 {
		_, err = exporter.SendContacts(ctx, &emailpb.SendContactsRequest{
			Emails: []string{"test@example.com"},
		})
		if err != nil {
			break
		}
	}
	test.AssertErrorIs(t, err, ErrQueueFull)
}

func TestSendContactsQueueDrains(t *testing.T) {
	t.Parallel()

	exporter, clientImpl, start, cleanup := setup()
	start()

	var emails []string
	for i := range 100 {
		emails = append(emails, fmt.Sprintf("test@%d.example.com", i))
	}

	_, err := exporter.SendContacts(ctx, &emailpb.SendContactsRequest{
		Emails: emails,
	})
	test.AssertNotError(t, err, "Error creating contacts")

	// Drain the queue.
	cleanup()

	test.AssertEquals(t, 100, len(clientImpl.getCreatedContacts()))
}

type mockAlwaysFailClient struct{}

func (m *mockAlwaysFailClient) SendContact(email string) error {
	return fmt.Errorf("simulated failure")
}

func TestSendContactsErrorMetrics(t *testing.T) {
	t.Parallel()

	mockClient := &mockAlwaysFailClient{}
	exporter := NewExporterImpl(mockClient, nil, 1000000, 5, metrics.NoopRegisterer, blog.NewMock())

	daemonCtx, cancel := context.WithCancel(context.Background())
	exporter.Start(daemonCtx)

	_, err := exporter.SendContacts(ctx, &emailpb.SendContactsRequest{
		Emails: []string{"test@example.com"},
	})
	test.AssertNotError(t, err, "Error creating contacts")

	// Drain the queue.
	cancel()
	exporter.Drain()

	// Check that the error counter was incremented.
	test.AssertMetricWithLabelsEquals(t, exporter.pardotErrorCounter, prometheus.Labels{}, 1)
}

func TestSendContactDeduplication(t *testing.T) {
	t.Parallel()

	cache := NewHashedEmailCache(1000, metrics.NoopRegisterer)
	mockClient, clientImpl := newMockPardotClientImpl()
	exporter := NewExporterImpl(mockClient, cache, 1000000, 5, metrics.NoopRegisterer, blog.NewMock())

	daemonCtx, cancel := context.WithCancel(context.Background())
	exporter.Start(daemonCtx)

	_, err := exporter.SendContacts(ctx, &emailpb.SendContactsRequest{
		Emails: []string{"duplicate@example.com", "duplicate@example.com"},
	})
	test.AssertNotError(t, err, "Error enqueuing contacts")

	// Drain the queue.
	cancel()
	exporter.Drain()

	contacts := clientImpl.getCreatedContacts()
	test.AssertEquals(t, 1, len(contacts))
	test.AssertEquals(t, "duplicate@example.com", contacts[0])

	// Only one successful send should be recorded.
	test.AssertMetricWithLabelsEquals(t, exporter.emailsHandledCounter, prometheus.Labels{}, 1)

	if !cache.Seen("duplicate@example.com") {
		t.Errorf("duplicate@example.com should have been cached after send")
	}
}

func TestSendContactErrorRemovesFromCache(t *testing.T) {
	t.Parallel()

	cache := NewHashedEmailCache(1000, metrics.NoopRegisterer)
	fc := &mockAlwaysFailClient{}

	exporter := NewExporterImpl(fc, cache, 1000000, 1, metrics.NoopRegisterer, blog.NewMock())

	daemonCtx, cancel := context.WithCancel(context.Background())
	exporter.Start(daemonCtx)

	_, err := exporter.SendContacts(ctx, &emailpb.SendContactsRequest{
		Emails: []string{"error@example.com"},
	})
	test.AssertNotError(t, err, "enqueue failed")

	// Drain the queue.
	cancel()
	exporter.Drain()

	// The email should have been evicted from the cache after send encountered
	// an error.
	if cache.Seen("error@example.com") {
		t.Errorf("error@example.com should have been evicted from cache after send errors")
	}

	// Check that the error counter was incremented.
	test.AssertMetricWithLabelsEquals(t, exporter.pardotErrorCounter, prometheus.Labels{}, 1)
}
