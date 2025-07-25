package ratelimits

import (
	"context"
	"math/rand/v2"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/jmhodges/clock"

	"github.com/letsencrypt/boulder/config"
	berrors "github.com/letsencrypt/boulder/errors"
	"github.com/letsencrypt/boulder/metrics"
	"github.com/letsencrypt/boulder/test"
)

// overriddenIP is overridden in 'testdata/working_override.yml' to have higher
// burst and count values.
const overriddenIP = "64.112.117.1"

// newTestLimiter constructs a new limiter.
func newTestLimiter(t *testing.T, s Source, clk clock.FakeClock) *Limiter {
	l, err := NewLimiter(clk, s, metrics.NoopRegisterer)
	test.AssertNotError(t, err, "should not error")
	return l
}

// newTestTransactionBuilder constructs a new *TransactionBuilder with the
// following configuration:
//   - 'NewRegistrationsPerIPAddress' burst: 20 count: 20 period: 1s
//   - 'NewRegistrationsPerIPAddress:64.112.117.1' burst: 40 count: 40 period: 1s
func newTestTransactionBuilder(t *testing.T) *TransactionBuilder {
	c, err := NewTransactionBuilderFromFiles("testdata/working_default.yml", "testdata/working_override.yml")
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
		randIP[i] = byte(rand.IntN(256))
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
			overriddenBucketKey := newIPAddressBucketKey(NewRegistrationsPerIPAddress, netip.MustParseAddr(overriddenIP))
			overriddenLimit, err := txnBuilder.getLimit(NewRegistrationsPerIPAddress, overriddenBucketKey)
			test.AssertNotError(t, err, "should not error")

			// Attempt to spend all 40 requests, this should succeed.
			overriddenTxn40, err := newTransaction(overriddenLimit, overriddenBucketKey, 40)
			test.AssertNotError(t, err, "txn should be valid")
			d, err := l.Spend(testCtx, overriddenTxn40)
			test.AssertNotError(t, err, "should not error")
			test.Assert(t, d.allowed, "should be allowed")

			// Attempting to spend 1 more, this should fail.
			overriddenTxn1, err := newTransaction(overriddenLimit, overriddenBucketKey, 1)
			test.AssertNotError(t, err, "txn should be valid")
			d, err = l.Spend(testCtx, overriddenTxn1)
			test.AssertNotError(t, err, "should not error")
			test.Assert(t, !d.allowed, "should not be allowed")
			test.AssertEquals(t, d.remaining, int64(0))
			test.AssertEquals(t, d.resetIn, time.Second)

			// Verify our RetryIn is correct. 1 second == 1000 milliseconds and
			// 1000/40 = 25 milliseconds per request.
			test.AssertEquals(t, d.retryIn, time.Millisecond*25)

			// Wait 50 milliseconds and try again.
			clk.Add(d.retryIn)

			// We should be allowed to spend 1 more request.
			d, err = l.Spend(testCtx, overriddenTxn1)
			test.AssertNotError(t, err, "should not error")
			test.Assert(t, d.allowed, "should be allowed")
			test.AssertEquals(t, d.remaining, int64(0))
			test.AssertEquals(t, d.resetIn, time.Second)

			// Wait 1 second for a full bucket reset.
			clk.Add(d.resetIn)

			// Quickly spend 40 requests in a row.
			for i := range 40 {
				d, err = l.Spend(testCtx, overriddenTxn1)
				test.AssertNotError(t, err, "should not error")
				test.Assert(t, d.allowed, "should be allowed")
				test.AssertEquals(t, d.remaining, int64(39-i))
			}

			// Attempting to spend 1 more, this should fail.
			d, err = l.Spend(testCtx, overriddenTxn1)
			test.AssertNotError(t, err, "should not error")
			test.Assert(t, !d.allowed, "should not be allowed")
			test.AssertEquals(t, d.remaining, int64(0))
			test.AssertEquals(t, d.resetIn, time.Second)

			// Wait 1 second for a full bucket reset.
			clk.Add(d.resetIn)

			normalBucketKey := newIPAddressBucketKey(NewRegistrationsPerIPAddress, netip.MustParseAddr(testIP))
			normalLimit, err := txnBuilder.getLimit(NewRegistrationsPerIPAddress, normalBucketKey)
			test.AssertNotError(t, err, "should not error")

			// Spend the same bucket but in a batch with bucket subject to
			// default limits. This should succeed, but the decision should
			// reflect that of the default bucket.
			defaultTxn1, err := newTransaction(normalLimit, normalBucketKey, 1)
			test.AssertNotError(t, err, "txn should be valid")
			d, err = l.BatchSpend(testCtx, []Transaction{overriddenTxn1, defaultTxn1})
			test.AssertNotError(t, err, "should not error")
			test.Assert(t, d.allowed, "should be allowed")
			test.AssertEquals(t, d.remaining, int64(19))
			test.AssertEquals(t, d.retryIn, time.Duration(0))
			test.AssertEquals(t, d.resetIn, time.Millisecond*50)

			// Refund quota to both buckets. This should succeed, but the
			// decision should reflect that of the default bucket.
			d, err = l.BatchRefund(testCtx, []Transaction{overriddenTxn1, defaultTxn1})
			test.AssertNotError(t, err, "should not error")
			test.Assert(t, d.allowed, "should be allowed")
			test.AssertEquals(t, d.remaining, int64(20))
			test.AssertEquals(t, d.retryIn, time.Duration(0))
			test.AssertEquals(t, d.resetIn, time.Duration(0))

			// Once more.
			d, err = l.BatchSpend(testCtx, []Transaction{overriddenTxn1, defaultTxn1})
			test.AssertNotError(t, err, "should not error")
			test.Assert(t, d.allowed, "should be allowed")
			test.AssertEquals(t, d.remaining, int64(19))
			test.AssertEquals(t, d.retryIn, time.Duration(0))
			test.AssertEquals(t, d.resetIn, time.Millisecond*50)

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
			test.AssertEquals(t, d.remaining, int64(19))
			test.AssertEquals(t, d.retryIn, time.Duration(0))
			test.AssertEquals(t, d.resetIn, time.Millisecond*50)

			// Check the remaining quota of the overridden bucket.
			overriddenCheckOnlyTxn0, err := newCheckOnlyTransaction(overriddenLimit, overriddenBucketKey, 0)
			test.AssertNotError(t, err, "txn should be valid")
			d, err = l.Check(testCtx, overriddenCheckOnlyTxn0)
			test.AssertNotError(t, err, "should not error")
			test.AssertEquals(t, d.remaining, int64(39))
			test.AssertEquals(t, d.retryIn, time.Duration(0))
			test.AssertEquals(t, d.resetIn, time.Millisecond*25)

			// Check the remaining quota of the default bucket.
			defaultTxn0, err := newTransaction(normalLimit, normalBucketKey, 0)
			test.AssertNotError(t, err, "txn should be valid")
			d, err = l.Check(testCtx, defaultTxn0)
			test.AssertNotError(t, err, "should not error")
			test.AssertEquals(t, d.remaining, int64(20))
			test.AssertEquals(t, d.retryIn, time.Duration(0))
			test.AssertEquals(t, d.resetIn, time.Duration(0))

			// Spend the same bucket but in a batch with a Transaction that is
			// spend-only. This should succeed, but the decision should reflect
			// that of the overridden bucket.
			defaultSpendOnlyTxn1, err := newSpendOnlyTransaction(normalLimit, normalBucketKey, 1)
			test.AssertNotError(t, err, "txn should be valid")
			d, err = l.BatchSpend(testCtx, []Transaction{overriddenTxn1, defaultSpendOnlyTxn1})
			test.AssertNotError(t, err, "should not error")
			test.AssertEquals(t, d.remaining, int64(38))
			test.AssertEquals(t, d.retryIn, time.Duration(0))
			test.AssertEquals(t, d.resetIn, time.Millisecond*50)

			// Check the remaining quota of the overridden bucket.
			d, err = l.Check(testCtx, overriddenCheckOnlyTxn0)
			test.AssertNotError(t, err, "should not error")
			test.AssertEquals(t, d.remaining, int64(38))
			test.AssertEquals(t, d.retryIn, time.Duration(0))
			test.AssertEquals(t, d.resetIn, time.Millisecond*50)

			// Check the remaining quota of the default bucket.
			d, err = l.Check(testCtx, defaultTxn0)
			test.AssertNotError(t, err, "should not error")
			test.AssertEquals(t, d.remaining, int64(19))
			test.AssertEquals(t, d.retryIn, time.Duration(0))
			test.AssertEquals(t, d.resetIn, time.Millisecond*50)

			// Once more, but in now the spend-only Transaction will attempt to
			// spend 20 requests. The spend-only Transaction should fail, but
			// the decision should reflect that of the overridden bucket.
			defaultSpendOnlyTxn20, err := newSpendOnlyTransaction(normalLimit, normalBucketKey, 20)
			test.AssertNotError(t, err, "txn should be valid")
			d, err = l.BatchSpend(testCtx, []Transaction{overriddenTxn1, defaultSpendOnlyTxn20})
			test.AssertNotError(t, err, "should not error")
			test.AssertEquals(t, d.remaining, int64(37))
			test.AssertEquals(t, d.retryIn, time.Duration(0))
			test.AssertEquals(t, d.resetIn, time.Millisecond*75)

			// Check the remaining quota of the overridden bucket.
			d, err = l.Check(testCtx, overriddenCheckOnlyTxn0)
			test.AssertNotError(t, err, "should not error")
			test.AssertEquals(t, d.remaining, int64(37))
			test.AssertEquals(t, d.retryIn, time.Duration(0))
			test.AssertEquals(t, d.resetIn, time.Millisecond*75)

			// Check the remaining quota of the default bucket.
			d, err = l.Check(testCtx, defaultTxn0)
			test.AssertNotError(t, err, "should not error")
			test.AssertEquals(t, d.remaining, int64(19))
			test.AssertEquals(t, d.retryIn, time.Duration(0))
			test.AssertEquals(t, d.resetIn, time.Millisecond*50)

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
			bucketKey := newIPAddressBucketKey(NewRegistrationsPerIPAddress, netip.MustParseAddr(testIP))
			limit, err := txnBuilder.getLimit(NewRegistrationsPerIPAddress, bucketKey)
			test.AssertNotError(t, err, "should not error")

			// Check on an empty bucket should return the theoretical next state
			// of that bucket if the cost were spent.
			txn1, err := newTransaction(limit, bucketKey, 1)
			test.AssertNotError(t, err, "txn should be valid")
			d, err := l.Check(testCtx, txn1)
			test.AssertNotError(t, err, "should not error")
			test.Assert(t, d.allowed, "should be allowed")
			test.AssertEquals(t, d.remaining, int64(19))
			// Verify our ResetIn timing is correct. 1 second == 1000
			// milliseconds and 1000/20 = 50 milliseconds per request.
			test.AssertEquals(t, d.resetIn, time.Millisecond*50)
			test.AssertEquals(t, d.retryIn, time.Duration(0))

			// However, that cost should not be spent yet, a 0 cost check should
			// tell us that we actually have 20 remaining.
			txn0, err := newTransaction(limit, bucketKey, 0)
			test.AssertNotError(t, err, "txn should be valid")
			d, err = l.Check(testCtx, txn0)
			test.AssertNotError(t, err, "should not error")
			test.Assert(t, d.allowed, "should be allowed")
			test.AssertEquals(t, d.remaining, int64(20))
			test.AssertEquals(t, d.resetIn, time.Duration(0))
			test.AssertEquals(t, d.retryIn, time.Duration(0))

			// Reset our bucket.
			err = l.Reset(testCtx, bucketKey)
			test.AssertNotError(t, err, "should not error")

			// Similar to above, but we'll use Spend() to actually initialize
			// the bucket. Spend should return the same result as Check.
			d, err = l.Spend(testCtx, txn1)
			test.AssertNotError(t, err, "should not error")
			test.Assert(t, d.allowed, "should be allowed")
			test.AssertEquals(t, d.remaining, int64(19))
			// Verify our ResetIn timing is correct. 1 second == 1000
			// milliseconds and 1000/20 = 50 milliseconds per request.
			test.AssertEquals(t, d.resetIn, time.Millisecond*50)
			test.AssertEquals(t, d.retryIn, time.Duration(0))

			// And that cost should have been spent; a 0 cost check should still
			// tell us that we have 19 remaining.
			d, err = l.Check(testCtx, txn0)
			test.AssertNotError(t, err, "should not error")
			test.Assert(t, d.allowed, "should be allowed")
			test.AssertEquals(t, d.remaining, int64(19))
			// Verify our ResetIn is correct. 1 second == 1000 milliseconds and
			// 1000/20 = 50 milliseconds per request.
			test.AssertEquals(t, d.resetIn, time.Millisecond*50)
			test.AssertEquals(t, d.retryIn, time.Duration(0))
		})
	}
}

func TestLimiter_DefaultLimits(t *testing.T) {
	t.Parallel()
	testCtx, limiters, txnBuilder, clk, testIP := setup(t)
	for name, l := range limiters {
		t.Run(name, func(t *testing.T) {
			bucketKey := newIPAddressBucketKey(NewRegistrationsPerIPAddress, netip.MustParseAddr(testIP))
			limit, err := txnBuilder.getLimit(NewRegistrationsPerIPAddress, bucketKey)
			test.AssertNotError(t, err, "should not error")

			// Attempt to spend all 20 requests, this should succeed.
			txn20, err := newTransaction(limit, bucketKey, 20)
			test.AssertNotError(t, err, "txn should be valid")
			d, err := l.Spend(testCtx, txn20)
			test.AssertNotError(t, err, "should not error")
			test.Assert(t, d.allowed, "should be allowed")
			test.AssertEquals(t, d.remaining, int64(0))
			test.AssertEquals(t, d.resetIn, time.Second)

			// Attempting to spend 1 more, this should fail.
			txn1, err := newTransaction(limit, bucketKey, 1)
			test.AssertNotError(t, err, "txn should be valid")
			d, err = l.Spend(testCtx, txn1)
			test.AssertNotError(t, err, "should not error")
			test.Assert(t, !d.allowed, "should not be allowed")
			test.AssertEquals(t, d.remaining, int64(0))
			test.AssertEquals(t, d.resetIn, time.Second)

			// Verify our ResetIn is correct. 1 second == 1000 milliseconds and
			// 1000/20 = 50 milliseconds per request.
			test.AssertEquals(t, d.retryIn, time.Millisecond*50)

			// Wait 50 milliseconds and try again.
			clk.Add(d.retryIn)

			// We should be allowed to spend 1 more request.
			d, err = l.Spend(testCtx, txn1)
			test.AssertNotError(t, err, "should not error")
			test.Assert(t, d.allowed, "should be allowed")
			test.AssertEquals(t, d.remaining, int64(0))
			test.AssertEquals(t, d.resetIn, time.Second)

			// Wait 1 second for a full bucket reset.
			clk.Add(d.resetIn)

			// Quickly spend 20 requests in a row.
			for i := range 20 {
				d, err = l.Spend(testCtx, txn1)
				test.AssertNotError(t, err, "should not error")
				test.Assert(t, d.allowed, "should be allowed")
				test.AssertEquals(t, d.remaining, int64(19-i))
			}

			// Attempting to spend 1 more, this should fail.
			d, err = l.Spend(testCtx, txn1)
			test.AssertNotError(t, err, "should not error")
			test.Assert(t, !d.allowed, "should not be allowed")
			test.AssertEquals(t, d.remaining, int64(0))
			test.AssertEquals(t, d.resetIn, time.Second)
		})
	}
}

func TestLimiter_RefundAndReset(t *testing.T) {
	t.Parallel()
	testCtx, limiters, txnBuilder, clk, testIP := setup(t)
	for name, l := range limiters {
		t.Run(name, func(t *testing.T) {
			bucketKey := newIPAddressBucketKey(NewRegistrationsPerIPAddress, netip.MustParseAddr(testIP))
			limit, err := txnBuilder.getLimit(NewRegistrationsPerIPAddress, bucketKey)
			test.AssertNotError(t, err, "should not error")

			// Attempt to spend all 20 requests, this should succeed.
			txn20, err := newTransaction(limit, bucketKey, 20)
			test.AssertNotError(t, err, "txn should be valid")
			d, err := l.Spend(testCtx, txn20)
			test.AssertNotError(t, err, "should not error")
			test.Assert(t, d.allowed, "should be allowed")
			test.AssertEquals(t, d.remaining, int64(0))
			test.AssertEquals(t, d.resetIn, time.Second)

			// Refund 10 requests.
			txn10, err := newTransaction(limit, bucketKey, 10)
			test.AssertNotError(t, err, "txn should be valid")
			d, err = l.Refund(testCtx, txn10)
			test.AssertNotError(t, err, "should not error")
			test.AssertEquals(t, d.remaining, int64(10))

			// Spend 10 requests, this should succeed.
			d, err = l.Spend(testCtx, txn10)
			test.AssertNotError(t, err, "should not error")
			test.Assert(t, d.allowed, "should be allowed")
			test.AssertEquals(t, d.remaining, int64(0))
			test.AssertEquals(t, d.resetIn, time.Second)

			err = l.Reset(testCtx, bucketKey)
			test.AssertNotError(t, err, "should not error")

			// Attempt to spend 20 more requests, this should succeed.
			d, err = l.Spend(testCtx, txn20)
			test.AssertNotError(t, err, "should not error")
			test.Assert(t, d.allowed, "should be allowed")
			test.AssertEquals(t, d.remaining, int64(0))
			test.AssertEquals(t, d.resetIn, time.Second)

			// Reset to full.
			clk.Add(d.resetIn)

			// Refund 1 requests above our limit, this should fail.
			txn1, err := newTransaction(limit, bucketKey, 1)
			test.AssertNotError(t, err, "txn should be valid")
			d, err = l.Refund(testCtx, txn1)
			test.AssertNotError(t, err, "should not error")
			test.Assert(t, !d.allowed, "should not be allowed")
			test.AssertEquals(t, d.remaining, int64(20))

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

func TestRateLimitError(t *testing.T) {
	t.Parallel()
	now := clock.NewFake().Now()

	testCases := []struct {
		name            string
		decision        *Decision
		expectedErr     string
		expectedErrType berrors.ErrorType
	}{
		{
			name: "Allowed decision",
			decision: &Decision{
				allowed: true,
			},
		},
		{
			name: "RegistrationsPerIP limit reached",
			decision: &Decision{
				allowed: false,
				retryIn: 5 * time.Second,
				transaction: Transaction{
					limit: &limit{
						name:   NewRegistrationsPerIPAddress,
						burst:  10,
						period: config.Duration{Duration: time.Hour},
					},
				},
			},
			expectedErr:     "too many new registrations (10) from this IP address in the last 1h0m0s, retry after 1970-01-01 00:00:05 UTC: see https://letsencrypt.org/docs/rate-limits/#new-registrations-per-ip-address",
			expectedErrType: berrors.RateLimit,
		},
		{
			name: "RegistrationsPerIPv6Range limit reached",
			decision: &Decision{
				allowed: false,
				retryIn: 10 * time.Second,
				transaction: Transaction{
					limit: &limit{
						name:   NewRegistrationsPerIPv6Range,
						burst:  5,
						period: config.Duration{Duration: time.Hour},
					},
				},
			},
			expectedErr:     "too many new registrations (5) from this /48 subnet of IPv6 addresses in the last 1h0m0s, retry after 1970-01-01 00:00:10 UTC: see https://letsencrypt.org/docs/rate-limits/#new-registrations-per-ipv6-range",
			expectedErrType: berrors.RateLimit,
		},
		{
			name: "NewOrdersPerAccount limit reached",
			decision: &Decision{
				allowed: false,
				retryIn: 10 * time.Second,
				transaction: Transaction{
					limit: &limit{
						name:   NewOrdersPerAccount,
						burst:  2,
						period: config.Duration{Duration: time.Hour},
					},
				},
			},
			expectedErr:     "too many new orders (2) from this account in the last 1h0m0s, retry after 1970-01-01 00:00:10 UTC: see https://letsencrypt.org/docs/rate-limits/#new-orders-per-account",
			expectedErrType: berrors.RateLimit,
		},
		{
			name: "FailedAuthorizationsPerDomainPerAccount limit reached",
			decision: &Decision{
				allowed: false,
				retryIn: 15 * time.Second,
				transaction: Transaction{
					limit: &limit{
						name:   FailedAuthorizationsPerDomainPerAccount,
						burst:  7,
						period: config.Duration{Duration: time.Hour},
					},
					bucketKey: "4:12345:example.com",
				},
			},
			expectedErr:     "too many failed authorizations (7) for \"example.com\" in the last 1h0m0s, retry after 1970-01-01 00:00:15 UTC: see https://letsencrypt.org/docs/rate-limits/#authorization-failures-per-hostname-per-account",
			expectedErrType: berrors.RateLimit,
		},
		{
			name: "CertificatesPerDomain limit reached",
			decision: &Decision{
				allowed: false,
				retryIn: 20 * time.Second,
				transaction: Transaction{
					limit: &limit{
						name:   CertificatesPerDomain,
						burst:  3,
						period: config.Duration{Duration: time.Hour},
					},
					bucketKey: "5:example.org",
				},
			},
			expectedErr:     "too many certificates (3) already issued for \"example.org\" in the last 1h0m0s, retry after 1970-01-01 00:00:20 UTC: see https://letsencrypt.org/docs/rate-limits/#new-certificates-per-registered-domain",
			expectedErrType: berrors.RateLimit,
		},
		{
			name: "CertificatesPerDomainPerAccount limit reached",
			decision: &Decision{
				allowed: false,
				retryIn: 20 * time.Second,
				transaction: Transaction{
					limit: &limit{
						name:   CertificatesPerDomainPerAccount,
						burst:  3,
						period: config.Duration{Duration: time.Hour},
					},
					bucketKey: "6:12345678:example.net",
				},
			},
			expectedErr:     "too many certificates (3) already issued for \"example.net\" in the last 1h0m0s, retry after 1970-01-01 00:00:20 UTC: see https://letsencrypt.org/docs/rate-limits/#new-certificates-per-registered-domain",
			expectedErrType: berrors.RateLimit,
		},
		{
			name: "Unknown rate limit name",
			decision: &Decision{
				allowed: false,
				retryIn: 30 * time.Second,
				transaction: Transaction{
					limit: &limit{
						name: 9999999,
					},
				},
			},
			expectedErr:     "cannot generate error for unknown rate limit",
			expectedErrType: berrors.InternalServer,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.decision.Result(now)
			if tc.expectedErr == "" {
				test.AssertNotError(t, err, "expected no error")
			} else {
				test.AssertError(t, err, "expected an error")
				test.AssertEquals(t, err.Error(), tc.expectedErr)
				test.AssertErrorIs(t, err, tc.expectedErrType)
			}
		})
	}
}
