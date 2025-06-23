package notmain

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	akamaipb "github.com/letsencrypt/boulder/akamai/proto"
	"github.com/letsencrypt/boulder/config"
	blog "github.com/letsencrypt/boulder/log"
	"github.com/letsencrypt/boulder/test"
)

func TestThroughput_optimizeAndValidate(t *testing.T) {
	dur := func(in time.Duration) config.Duration { return config.Duration{Duration: in} }

	tests := []struct {
		name    string
		input   Throughput
		want    Throughput
		wantErr string
	}{
		{
			"negative instances",
			Throughput{defaultEntriesPerBatch, dur(defaultPurgeBatchInterval), -1},
			Throughput{},
			"must be positive",
		},
		{
			"negative batch interval",
			Throughput{defaultEntriesPerBatch, config.Duration{Duration: -1}, -1},
			Throughput{},
			"must be positive",
		},
		{
			"negative entries per batch",
			Throughput{-1, dur(defaultPurgeBatchInterval), 1},
			Throughput{},
			"must be positive",
		},
		{
			"empty input computes sane defaults",
			Throughput{},
			Throughput{defaultEntriesPerBatch, dur(defaultPurgeBatchInterval), 1},
			"",
		},
		{
			"strict configuration is honored",
			Throughput{2, dur(1 * time.Second), 1},
			Throughput{2, dur(1 * time.Second), 1},
			"",
		},
		{
			"slightly looser configuration still within limits",
			Throughput{defaultEntriesPerBatch, dur(defaultPurgeBatchInterval - time.Millisecond), 1},
			Throughput{defaultEntriesPerBatch, dur(defaultPurgeBatchInterval - time.Millisecond), 1},
			"",
		},
		{
			"too many requests per second",
			Throughput{QueueEntriesPerBatch: 1, PurgeBatchInterval: dur(19999 * time.Microsecond)},
			Throughput{},
			"requests per second limit",
		},
		{
			"too many URLs per second",
			Throughput{PurgeBatchInterval: dur(29 * time.Millisecond)},
			Throughput{},
			"URLs per second limit",
		},
		{
			"too many bytes per request",
			Throughput{QueueEntriesPerBatch: 125, PurgeBatchInterval: dur(1 * time.Second)},
			Throughput{},
			"bytes per request limit",
		},
		{
			"two instances computes sane defaults",
			Throughput{TotalInstances: 2},
			Throughput{defaultEntriesPerBatch, dur(defaultPurgeBatchInterval * 2), 2},
			"",
		},
		{
			"too many requests per second across multiple instances",
			Throughput{PurgeBatchInterval: dur(defaultPurgeBatchInterval), TotalInstances: 2},
			Throughput{},
			"requests per second limit",
		},
		{
			"too many entries per second across multiple instances",
			Throughput{PurgeBatchInterval: dur(59 * time.Millisecond), TotalInstances: 2},
			Throughput{},
			"URLs per second limit",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.input.optimizeAndValidate()
			if tc.wantErr != "" {
				test.AssertError(t, err, "")
				test.AssertContains(t, err.Error(), tc.wantErr)
			} else {
				test.AssertNotError(t, err, "")
				test.AssertEquals(t, tc.input, tc.want)
			}
		})
	}
}

type mockCCU struct {
	akamaipb.AkamaiPurgerClient
}

func (m *mockCCU) Purge(urls []string) error {
	return errors.New("Lol, I'm a mock")
}

func TestAkamaiPurgerQueue(t *testing.T) {
	ap := &akamaiPurger{
		maxStackSize:    250,
		entriesPerBatch: 2,
		client:          &mockCCU{},
		log:             blog.NewMock(),
	}

	// Add 250 entries to fill the stack.
	for i := range 250 {
		req := akamaipb.PurgeRequest{Urls: []string{fmt.Sprintf("http://test.com/%d", i)}}
		_, err := ap.Purge(context.Background(), &req)
		test.AssertNotError(t, err, fmt.Sprintf("Purge failed for entry %d.", i))
	}

	// Add another entry to the stack and using the Purge method.
	req := akamaipb.PurgeRequest{Urls: []string{"http://test.com/250"}}
	_, err := ap.Purge(context.Background(), &req)
	test.AssertNotError(t, err, "Purge failed.")

	// Verify that the stack is still full.
	test.AssertEquals(t, len(ap.toPurge), 250)

	// Verify that the first entry in the stack is the entry we just added.
	test.AssertEquals(t, ap.toPurge[len(ap.toPurge)-1][0], "http://test.com/250")

	// Verify that the last entry in the stack is the second entry we added.
	test.AssertEquals(t, ap.toPurge[0][0], "http://test.com/1")

	expectedTopEntryAfterFailure := ap.toPurge[len(ap.toPurge)-(ap.entriesPerBatch+1)][0]

	// Fail to purge a batch of entries from the stack.
	batch := ap.takeBatch()
	test.AssertNotNil(t, batch, "Batch should not be nil.")

	err = ap.purgeBatch(batch)
	test.AssertError(t, err, "Mock should have failed to purge.")

	// Verify that the stack is no longer full.
	test.AssertEquals(t, len(ap.toPurge), 248)

	// The first entry of the next batch should be on the top after the failed
	// purge.
	test.AssertEquals(t, ap.toPurge[len(ap.toPurge)-1][0], expectedTopEntryAfterFailure)
}

func TestAkamaiPurgerQueueWithOneEntry(t *testing.T) {
	ap := &akamaiPurger{
		maxStackSize:    250,
		entriesPerBatch: 2,
		client:          &mockCCU{},
		log:             blog.NewMock(),
	}

	// Add one entry to the stack and using the Purge method.
	req := akamaipb.PurgeRequest{Urls: []string{"http://test.com/0"}}
	_, err := ap.Purge(context.Background(), &req)
	test.AssertNotError(t, err, "Purge failed.")
	test.AssertEquals(t, len(ap.toPurge), 1)
	test.AssertEquals(t, ap.toPurge[len(ap.toPurge)-1][0], "http://test.com/0")

	// Fail to purge a batch of entries from the stack.
	batch := ap.takeBatch()
	test.AssertNotNil(t, batch, "Batch should not be nil.")

	err = ap.purgeBatch(batch)
	test.AssertError(t, err, "Mock should have failed to purge.")

	// Verify that the stack no longer contains our entry.
	test.AssertEquals(t, len(ap.toPurge), 0)
}
