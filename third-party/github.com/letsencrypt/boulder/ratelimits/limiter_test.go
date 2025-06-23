package ratelimits

import (
	"context"
	"math/rand"
	"net"
	"testing"
	"time"

	"github.com/jmhodges/clock"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/letsencrypt/boulder/metrics"
	"github.com/letsencrypt/boulder/test"
)

// tenZeroZeroTwo is overridden in 'testdata/working_override.yml' to have
// higher burst and count values.
const tenZeroZeroTwo = "10.0.0.2"

// newTestLimiter constructs a new limiter.
func newTestLimiter(t *testing.T, s source, clk clock.FakeClock) *Limiter {
	l, err := NewLimiter(clk, s, metrics.NoopRegisterer)
	test.AssertNotError(t, err, "should not error")
	return l
}

// newTestTransactionBuilder constructs a new *TransactionBuilder with the
// following configuration:
//   - 'NewRegistrationsPerIPAddress' burst: 20 count: 20 period: 1s
//   - 'NewRegistrationsPerIPAddress:10.0.0.2' burst: 40 count: 40 period: 1s
func newTestTransactionBuilder(t *testing.T) *TransactionBuilder {
	c, err := NewTransactionBuilder("testdata/working_default.yml", "testdata/working_override.yml")
	test.AssertNotError(t, err, "should not error")
	return c
}

func setup(t *testing.T) (context.Context, map[string]*Limiter, *TransactionBuilder, clock.FakeClock, string) {
	testCtx := context.Background()
	clk := clock.NewFake()

	// Generate a random IP address to avoid collisions during and between test
	// runs.
	randIP := make(net.IP, 4)
	for i := range 4 {
		randIP[i] = byte(rand.Intn(256))
	}

	// Construct a limiter for each source.
	return testCtx, map[string]*Limiter{
		"inmem": newInmemTestLimiter(t, clk),
		"redis": newRedisTestLimiter(t, clk),
	}, newTestTransactionBuilder(t), clk, randIP.String()
}

func TestLimiter_CheckWithLimitOverrides(t *testing.T) {
	t.Parallel()
	testCtx, limiters, txnBuilder, clk, testIP := setup(t)
	for name, l := range limiters {
		t.Run(name, func(t *testing.T) {
			// Verify our overrideUsageGauge is being set correctly. 0.0 == 0%
			// of the bucket has been consumed.
			test.AssertMetricWithLabelsEquals(t, l.overrideUsageGauge, prometheus.Labels{
				"limit":      NewRegistrationsPerIPAddress.String(),
				"bucket_key": joinWithColon(NewRegistrationsPerIPAddress.EnumString(), tenZeroZeroTwo)}, 0)

			overriddenBucketKey, err := newIPAddressBucketKey(NewRegistrationsPerIPAddress, net.ParseIP(tenZeroZeroTwo))
			test.AssertNotError(t, err, "should not error")
			overriddenLimit, err := txnBuilder.getLimit(NewRegistrationsPerIPAddress, overriddenBucketKey)
			test.AssertNotError(t, err, "should not error")

			// Attempt to spend all 40 requests, this should succeed.
			overriddenTxn40, err := newTransaction(overriddenLimit, overriddenBucketKey, 40)
			test.AssertNotError(t, err, "txn should be valid")
			d, err := l.Spend(testCtx, overriddenTxn40)
			test.AssertNotError(t, err, "should not error")
			test.Assert(t, d.Allowed, "should be allowed")

			// Attempting to spend 1 more, this should fail.
			overriddenTxn1, err := newTransaction(overriddenLimit, overriddenBucketKey, 1)
			test.AssertNotError(t, err, "txn should be valid")
			d, err = l.Spend(testCtx, overriddenTxn1)
			test.AssertNotError(t, err, "should not error")
			test.Assert(t, !d.Allowed, "should not be allowed")
			test.AssertEquals(t, d.Remaining, int64(0))
			test.AssertEquals(t, d.ResetIn, time.Second)

			// Verify our overrideUsageGauge is being set correctly. 1.0 == 100%
			// of the bucket has been consumed.
			test.AssertMetricWithLabelsEquals(t, l.overrideUsageGauge, prometheus.Labels{
				"limit_name": NewRegistrationsPerIPAddress.String(),
				"bucket_key": joinWithColon(NewRegistrationsPerIPAddress.EnumString(), tenZeroZeroTwo)}, 1.0)

			// Verify our RetryIn is correct. 1 second == 1000 milliseconds and
			// 1000/40 = 25 milliseconds per request.
			test.AssertEquals(t, d.RetryIn, time.Millisecond*25)

			// Wait 50 milliseconds and try again.
			clk.Add(d.RetryIn)

			// We should be allowed to spend 1 more request.
			d, err = l.Spend(testCtx, overriddenTxn1)
			test.AssertNotError(t, err, "should not error")
			test.Assert(t, d.Allowed, "should be allowed")
			test.AssertEquals(t, d.Remaining, int64(0))
			test.AssertEquals(t, d.ResetIn, time.Second)

			// Wait 1 second for a full bucket reset.
			clk.Add(d.ResetIn)

			// Quickly spend 40 requests in a row.
			for i := range 40 {
				d, err = l.Spend(testCtx, overriddenTxn1)
				test.AssertNotError(t, err, "should not error")
				test.Assert(t, d.Allowed, "should be allowed")
				test.AssertEquals(t, d.Remaining, int64(39-i))
			}

			// Attempting to spend 1 more, this should fail.
			d, err = l.Spend(testCtx, overriddenTxn1)
			test.AssertNotError(t, err, "should not error")
			test.Assert(t, !d.Allowed, "should not be allowed")
			test.AssertEquals(t, d.Remaining, int64(0))
			test.AssertEquals(t, d.ResetIn, time.Second)

			// Wait 1 second for a full bucket reset.
			clk.Add(d.ResetIn)

			testIP := net.ParseIP(testIP)
			normalBucketKey, err := newIPAddressBucketKey(NewRegistrationsPerIPAddress, testIP)
			test.AssertNotError(t, err, "should not error")
			normalLimit, err := txnBuilder.getLimit(NewRegistrationsPerIPAddress, normalBucketKey)
			test.AssertNotError(t, err, "should not error")

			// Spend the same bucket but in a batch with bucket subject to
			// default limits. This should succeed, but the decision should
			// reflect that of the default bucket.
			defaultTxn1, err := newTransaction(normalLimit, normalBucketKey, 1)
			test.AssertNotError(t, err, "txn should be valid")
			d, err = l.BatchSpend(testCtx, []Transaction{overriddenTxn1, defaultTxn1})
			test.AssertNotError(t, err, "should not error")
			test.Assert(t, d.Allowed, "should be allowed")
			test.AssertEquals(t, d.Remaining, int64(19))
			test.AssertEquals(t, d.RetryIn, time.Duration(0))
			test.AssertEquals(t, d.ResetIn, time.Millisecond*50)

			// Refund quota to both buckets. This should succeed, but the
			// decision should reflect that of the default bucket.
			d, err = l.BatchRefund(testCtx, []Transaction{overriddenTxn1, defaultTxn1})
			test.AssertNotError(t, err, "should not error")
			test.Assert(t, d.Allowed, "should be allowed")
			test.AssertEquals(t, d.Remaining, int64(20))
			test.AssertEquals(t, d.RetryIn, time.Duration(0))
			test.AssertEquals(t, d.ResetIn, time.Duration(0))

			// Once more.
			d, err = l.BatchSpend(testCtx, []Transaction{overriddenTxn1, defaultTxn1})
			test.AssertNotError(t, err, "should not error")
			test.Assert(t, d.Allowed, "should be allowed")
			test.AssertEquals(t, d.Remaining, int64(19))
			test.AssertEquals(t, d.RetryIn, time.Duration(0))
			test.AssertEquals(t, d.ResetIn, time.Millisecond*50)

			// Reset between tests.
			err = l.Reset(testCtx, overriddenBucketKey)
			test.AssertNotError(t, err, "should not error")
			err = l.Reset(testCtx, normalBucketKey)
			test.AssertNotError(t, err, "should not error")

			// Spend the same bucket but in a batch with a Transaction that is
			// check-only. This should succeed, but the decision should reflect
			// that of the default bucket.
			defaultCheckOnlyTxn1, err := newCheckOnlyTransaction(normalLimit, normalBucketKey, 1)
			test.AssertNotError(t, err, "txn should be valid")
			d, err = l.BatchSpend(testCtx, []Transaction{overriddenTxn1, defaultCheckOnlyTxn1})
			test.AssertNotError(t, err, "should not error")
			test.AssertEquals(t, d.Remaining, int64(19))
			test.AssertEquals(t, d.RetryIn, time.Duration(0))
			test.AssertEquals(t, d.ResetIn, time.Millisecond*50)

			// Check the remaining quota of the overridden bucket.
			overriddenCheckOnlyTxn0, err := newCheckOnlyTransaction(overriddenLimit, overriddenBucketKey, 0)
			test.AssertNotError(t, err, "txn should be valid")
			d, err = l.Check(testCtx, overriddenCheckOnlyTxn0)
			test.AssertNotError(t, err, "should not error")
			test.AssertEquals(t, d.Remaining, int64(39))
			test.AssertEquals(t, d.RetryIn, time.Duration(0))
			test.AssertEquals(t, d.ResetIn, time.Millisecond*25)

			// Check the remaining quota of the default bucket.
			defaultTxn0, err := newTransaction(normalLimit, normalBucketKey, 0)
			test.AssertNotError(t, err, "txn should be valid")
			d, err = l.Check(testCtx, defaultTxn0)
			test.AssertNotError(t, err, "should not error")
			test.AssertEquals(t, d.Remaining, int64(20))
			test.AssertEquals(t, d.RetryIn, time.Duration(0))
			test.AssertEquals(t, d.ResetIn, time.Duration(0))

			// Spend the same bucket but in a batch with a Transaction that is
			// spend-only. This should succeed, but the decision should reflect
			// that of the overridden bucket.
			defaultSpendOnlyTxn1, err := newSpendOnlyTransaction(normalLimit, normalBucketKey, 1)
			test.AssertNotError(t, err, "txn should be valid")
			d, err = l.BatchSpend(testCtx, []Transaction{overriddenTxn1, defaultSpendOnlyTxn1})
			test.AssertNotError(t, err, "should not error")
			test.AssertEquals(t, d.Remaining, int64(38))
			test.AssertEquals(t, d.RetryIn, time.Duration(0))
			test.AssertEquals(t, d.ResetIn, time.Millisecond*50)

			// Check the remaining quota of the overridden bucket.
			d, err = l.Check(testCtx, overriddenCheckOnlyTxn0)
			test.AssertNotError(t, err, "should not error")
			test.AssertEquals(t, d.Remaining, int64(38))
			test.AssertEquals(t, d.RetryIn, time.Duration(0))
			test.AssertEquals(t, d.ResetIn, time.Millisecond*50)

			// Check the remaining quota of the default bucket.
			d, err = l.Check(testCtx, defaultTxn0)
			test.AssertNotError(t, err, "should not error")
			test.AssertEquals(t, d.Remaining, int64(19))
			test.AssertEquals(t, d.RetryIn, time.Duration(0))
			test.AssertEquals(t, d.ResetIn, time.Millisecond*50)

			// Once more, but in now the spend-only Transaction will attempt to
			// spend 20 requests. The spend-only Transaction should fail, but
			// the decision should reflect that of the overridden bucket.
			defaultSpendOnlyTxn20, err := newSpendOnlyTransaction(normalLimit, normalBucketKey, 20)
			test.AssertNotError(t, err, "txn should be valid")
			d, err = l.BatchSpend(testCtx, []Transaction{overriddenTxn1, defaultSpendOnlyTxn20})
			test.AssertNotError(t, err, "should not error")
			test.AssertEquals(t, d.Remaining, int64(37))
			test.AssertEquals(t, d.RetryIn, time.Duration(0))
			test.AssertEquals(t, d.ResetIn, time.Millisecond*75)

			// Check the remaining quota of the overridden bucket.
			d, err = l.Check(testCtx, overriddenCheckOnlyTxn0)
			test.AssertNotError(t, err, "should not error")
			test.AssertEquals(t, d.Remaining, int64(37))
			test.AssertEquals(t, d.RetryIn, time.Duration(0))
			test.AssertEquals(t, d.ResetIn, time.Millisecond*75)

			// Check the remaining quota of the default bucket.
			d, err = l.Check(testCtx, defaultTxn0)
			test.AssertNotError(t, err, "should not error")
			test.AssertEquals(t, d.Remaining, int64(19))
			test.AssertEquals(t, d.RetryIn, time.Duration(0))
			test.AssertEquals(t, d.ResetIn, time.Millisecond*50)

			// Reset between tests.
			err = l.Reset(testCtx, overriddenBucketKey)
			test.AssertNotError(t, err, "should not error")
		})
	}
}

func TestLimiter_InitializationViaCheckAndSpend(t *testing.T) {
	t.Parallel()
	testCtx, limiters, txnBuilder, _, testIP := setup(t)
	for name, l := range limiters {
		t.Run(name, func(t *testing.T) {
			bucketKey, err := newIPAddressBucketKey(NewRegistrationsPerIPAddress, net.ParseIP(testIP))
			test.AssertNotError(t, err, "should not error")
			limit, err := txnBuilder.getLimit(NewRegistrationsPerIPAddress, bucketKey)
			test.AssertNotError(t, err, "should not error")

			// Check on an empty bucket should return the theoretical next state
			// of that bucket if the cost were spent.
			txn1, err := newTransaction(limit, bucketKey, 1)
			test.AssertNotError(t, err, "txn should be valid")
			d, err := l.Check(testCtx, txn1)
			test.AssertNotError(t, err, "should not error")
			test.Assert(t, d.Allowed, "should be allowed")
			test.AssertEquals(t, d.Remaining, int64(19))
			// Verify our ResetIn timing is correct. 1 second == 1000
			// milliseconds and 1000/20 = 50 milliseconds per request.
			test.AssertEquals(t, d.ResetIn, time.Millisecond*50)
			test.AssertEquals(t, d.RetryIn, time.Duration(0))

			// However, that cost should not be spent yet, a 0 cost check should
			// tell us that we actually have 20 remaining.
			txn0, err := newTransaction(limit, bucketKey, 0)
			test.AssertNotError(t, err, "txn should be valid")
			d, err = l.Check(testCtx, txn0)
			test.AssertNotError(t, err, "should not error")
			test.Assert(t, d.Allowed, "should be allowed")
			test.AssertEquals(t, d.Remaining, int64(20))
			test.AssertEquals(t, d.ResetIn, time.Duration(0))
			test.AssertEquals(t, d.RetryIn, time.Duration(0))

			// Reset our bucket.
			err = l.Reset(testCtx, bucketKey)
			test.AssertNotError(t, err, "should not error")

			// Similar to above, but we'll use Spend() to actually initialize
			// the bucket. Spend should return the same result as Check.
			d, err = l.Spend(testCtx, txn1)
			test.AssertNotError(t, err, "should not error")
			test.Assert(t, d.Allowed, "should be allowed")
			test.AssertEquals(t, d.Remaining, int64(19))
			// Verify our ResetIn timing is correct. 1 second == 1000
			// milliseconds and 1000/20 = 50 milliseconds per request.
			test.AssertEquals(t, d.ResetIn, time.Millisecond*50)
			test.AssertEquals(t, d.RetryIn, time.Duration(0))

			// However, that cost should not be spent yet, a 0 cost check should
			// tell us that we actually have 19 remaining.
			d, err = l.Check(testCtx, txn0)
			test.AssertNotError(t, err, "should not error")
			test.Assert(t, d.Allowed, "should be allowed")
			test.AssertEquals(t, d.Remaining, int64(19))
			// Verify our ResetIn is correct. 1 second == 1000 milliseconds and
			// 1000/20 = 50 milliseconds per request.
			test.AssertEquals(t, d.ResetIn, time.Millisecond*50)
			test.AssertEquals(t, d.RetryIn, time.Duration(0))
		})
	}
}

func TestLimiter_DefaultLimits(t *testing.T) {
	t.Parallel()
	testCtx, limiters, txnBuilder, clk, testIP := setup(t)
	for name, l := range limiters {
		t.Run(name, func(t *testing.T) {
			bucketKey, err := newIPAddressBucketKey(NewRegistrationsPerIPAddress, net.ParseIP(testIP))
			test.AssertNotError(t, err, "should not error")
			limit, err := txnBuilder.getLimit(NewRegistrationsPerIPAddress, bucketKey)
			test.AssertNotError(t, err, "should not error")

			// Attempt to spend all 20 requests, this should succeed.
			txn20, err := newTransaction(limit, bucketKey, 20)
			test.AssertNotError(t, err, "txn should be valid")
			d, err := l.Spend(testCtx, txn20)
			test.AssertNotError(t, err, "should not error")
			test.Assert(t, d.Allowed, "should be allowed")
			test.AssertEquals(t, d.Remaining, int64(0))
			test.AssertEquals(t, d.ResetIn, time.Second)

			// Attempting to spend 1 more, this should fail.
			txn1, err := newTransaction(limit, bucketKey, 1)
			test.AssertNotError(t, err, "txn should be valid")
			d, err = l.Spend(testCtx, txn1)
			test.AssertNotError(t, err, "should not error")
			test.Assert(t, !d.Allowed, "should not be allowed")
			test.AssertEquals(t, d.Remaining, int64(0))
			test.AssertEquals(t, d.ResetIn, time.Second)

			// Verify our ResetIn is correct. 1 second == 1000 milliseconds and
			// 1000/20 = 50 milliseconds per request.
			test.AssertEquals(t, d.RetryIn, time.Millisecond*50)

			// Wait 50 milliseconds and try again.
			clk.Add(d.RetryIn)

			// We should be allowed to spend 1 more request.
			d, err = l.Spend(testCtx, txn1)
			test.AssertNotError(t, err, "should not error")
			test.Assert(t, d.Allowed, "should be allowed")
			test.AssertEquals(t, d.Remaining, int64(0))
			test.AssertEquals(t, d.ResetIn, time.Second)

			// Wait 1 second for a full bucket reset.
			clk.Add(d.ResetIn)

			// Quickly spend 20 requests in a row.
			for i := range 20 {
				d, err = l.Spend(testCtx, txn1)
				test.AssertNotError(t, err, "should not error")
				test.Assert(t, d.Allowed, "should be allowed")
				test.AssertEquals(t, d.Remaining, int64(19-i))
			}

			// Attempting to spend 1 more, this should fail.
			d, err = l.Spend(testCtx, txn1)
			test.AssertNotError(t, err, "should not error")
			test.Assert(t, !d.Allowed, "should not be allowed")
			test.AssertEquals(t, d.Remaining, int64(0))
			test.AssertEquals(t, d.ResetIn, time.Second)
		})
	}
}

func TestLimiter_RefundAndReset(t *testing.T) {
	t.Parallel()
	testCtx, limiters, txnBuilder, clk, testIP := setup(t)
	for name, l := range limiters {
		t.Run(name, func(t *testing.T) {
			bucketKey, err := newIPAddressBucketKey(NewRegistrationsPerIPAddress, net.ParseIP(testIP))
			test.AssertNotError(t, err, "should not error")
			limit, err := txnBuilder.getLimit(NewRegistrationsPerIPAddress, bucketKey)
			test.AssertNotError(t, err, "should not error")

			// Attempt to spend all 20 requests, this should succeed.
			txn20, err := newTransaction(limit, bucketKey, 20)
			test.AssertNotError(t, err, "txn should be valid")
			d, err := l.Spend(testCtx, txn20)
			test.AssertNotError(t, err, "should not error")
			test.Assert(t, d.Allowed, "should be allowed")
			test.AssertEquals(t, d.Remaining, int64(0))
			test.AssertEquals(t, d.ResetIn, time.Second)

			// Refund 10 requests.
			txn10, err := newTransaction(limit, bucketKey, 10)
			test.AssertNotError(t, err, "txn should be valid")
			d, err = l.Refund(testCtx, txn10)
			test.AssertNotError(t, err, "should not error")
			test.AssertEquals(t, d.Remaining, int64(10))

			// Spend 10 requests, this should succeed.
			d, err = l.Spend(testCtx, txn10)
			test.AssertNotError(t, err, "should not error")
			test.Assert(t, d.Allowed, "should be allowed")
			test.AssertEquals(t, d.Remaining, int64(0))
			test.AssertEquals(t, d.ResetIn, time.Second)

			err = l.Reset(testCtx, bucketKey)
			test.AssertNotError(t, err, "should not error")

			// Attempt to spend 20 more requests, this should succeed.
			d, err = l.Spend(testCtx, txn20)
			test.AssertNotError(t, err, "should not error")
			test.Assert(t, d.Allowed, "should be allowed")
			test.AssertEquals(t, d.Remaining, int64(0))
			test.AssertEquals(t, d.ResetIn, time.Second)

			// Reset to full.
			clk.Add(d.ResetIn)

			// Refund 1 requests above our limit, this should fail.
			txn1, err := newTransaction(limit, bucketKey, 1)
			test.AssertNotError(t, err, "txn should be valid")
			d, err = l.Refund(testCtx, txn1)
			test.AssertNotError(t, err, "should not error")
			test.Assert(t, !d.Allowed, "should not be allowed")
			test.AssertEquals(t, d.Remaining, int64(20))

			// Spend so we can refund.
			_, err = l.Spend(testCtx, txn1)
			test.AssertNotError(t, err, "should not error")

			// Refund a spendOnly Transaction, which should succeed.
			spendOnlyTxn1, err := newSpendOnlyTransaction(limit, bucketKey, 1)
			test.AssertNotError(t, err, "txn should be valid")
			_, err = l.Refund(testCtx, spendOnlyTxn1)
			test.AssertNotError(t, err, "should not error")

			// Spend so we can refund.
			expectedDecision, err := l.Spend(testCtx, txn1)
			test.AssertNotError(t, err, "should not error")

			// Refund a checkOnly Transaction, which shouldn't error but should
			// return the same TAT as the previous spend.
			checkOnlyTxn1, err := newCheckOnlyTransaction(limit, bucketKey, 1)
			test.AssertNotError(t, err, "txn should be valid")
			newDecision, err := l.Refund(testCtx, checkOnlyTxn1)
			test.AssertNotError(t, err, "should not error")
			test.AssertEquals(t, newDecision.newTAT, expectedDecision.newTAT)
		})
	}
}
