package ratelimits

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/jmhodges/clock"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	// Allowed is used for rate limit metrics, it's the value of the 'decision'
	// label when a request was allowed.
	Allowed = "allowed"

	// Denied is used for rate limit metrics, it's the value of the 'decision'
	// label when a request was denied.
	Denied = "denied"
)

// allowedDecision is an "allowed" *Decision that should be returned when a
// checked limit is found to be disabled.
var allowedDecision = &Decision{Allowed: true, Remaining: math.MaxInt64}

// Limiter provides a high-level interface for rate limiting requests by
// utilizing a leaky bucket-style approach.
type Limiter struct {
	// source is used to store buckets. It must be safe for concurrent use.
	source source
	clk    clock.Clock

	spendLatency       *prometheus.HistogramVec
	overrideUsageGauge *prometheus.GaugeVec
}

// NewLimiter returns a new *Limiter. The provided source must be safe for
// concurrent use.
func NewLimiter(clk clock.Clock, source source, stats prometheus.Registerer) (*Limiter, error) {
	limiter := &Limiter{source: source, clk: clk}
	limiter.spendLatency = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "ratelimits_spend_latency",
		Help: fmt.Sprintf("Latency of ratelimit checks labeled by limit=[name] and decision=[%s|%s], in seconds", Allowed, Denied),
		// Exponential buckets ranging from 0.0005s to 3s.
		Buckets: prometheus.ExponentialBuckets(0.0005, 3, 8),
	}, []string{"limit", "decision"})
	stats.MustRegister(limiter.spendLatency)

	limiter.overrideUsageGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ratelimits_override_usage",
		Help: "Proportion of override limit used, by limit name and bucket key.",
	}, []string{"limit", "bucket_key"})
	stats.MustRegister(limiter.overrideUsageGauge)

	return limiter, nil
}

type Decision struct {
	// Allowed is true if the bucket possessed enough capacity to allow the
	// request given the cost.
	Allowed bool

	// Remaining is the number of requests the client is allowed to make before
	// they're rate limited.
	Remaining int64

	// RetryIn is the duration the client MUST wait before they're allowed to
	// make a request.
	RetryIn time.Duration

	// ResetIn is the duration the bucket will take to refill to its maximum
	// capacity, assuming no further requests are made.
	ResetIn time.Duration

	// newTAT indicates the time at which the bucket will be full. It is the
	// theoretical arrival time (TAT) of next request. It must be no more than
	// (burst * (period / count)) in the future at any single point in time.
	newTAT time.Time
}

// Check DOES NOT deduct the cost of the request from the provided bucket's
// capacity. The returned *Decision indicates whether the capacity exists to
// satisfy the cost and represents the hypothetical state of the bucket IF the
// cost WERE to be deducted. If no bucket exists it will NOT be created. No
// state is persisted to the underlying datastore.
func (l *Limiter) Check(ctx context.Context, txn Transaction) (*Decision, error) {
	if txn.allowOnly() {
		return allowedDecision, nil
	}
	// Remove cancellation from the request context so that transactions are not
	// interrupted by a client disconnect.
	ctx = context.WithoutCancel(ctx)
	tat, err := l.source.Get(ctx, txn.bucketKey)
	if err != nil {
		if !errors.Is(err, ErrBucketNotFound) {
			return nil, err
		}
		// First request from this client. No need to initialize the bucket
		// because this is a check, not a spend. A TAT of "now" is equivalent to
		// a full bucket.
		return maybeSpend(l.clk, txn.limit, l.clk.Now(), txn.cost), nil
	}
	return maybeSpend(l.clk, txn.limit, tat, txn.cost), nil
}

// Spend attempts to deduct the cost from the provided bucket's capacity. The
// returned *Decision indicates whether the capacity existed to satisfy the cost
// and represents the current state of the bucket. If no bucket exists it WILL
// be created WITH the cost factored into its initial state. The new bucket
// state is persisted to the underlying datastore, if applicable, before
// returning.
func (l *Limiter) Spend(ctx context.Context, txn Transaction) (*Decision, error) {
	return l.BatchSpend(ctx, []Transaction{txn})
}

func prepareBatch(txns []Transaction) ([]Transaction, []string, error) {
	var bucketKeys []string
	var transactions []Transaction
	for _, txn := range txns {
		if txn.allowOnly() {
			// Ignore allow-only transactions.
			continue
		}
		if slices.Contains(bucketKeys, txn.bucketKey) {
			return nil, nil, fmt.Errorf("found duplicate bucket %q in batch", txn.bucketKey)
		}
		bucketKeys = append(bucketKeys, txn.bucketKey)
		transactions = append(transactions, txn)
	}
	return transactions, bucketKeys, nil
}

type batchDecision struct {
	*Decision
}

func newBatchDecision() *batchDecision {
	return &batchDecision{
		Decision: &Decision{
			Allowed:   true,
			Remaining: math.MaxInt64,
		},
	}
}

func (d *batchDecision) merge(in *Decision) {
	d.Allowed = d.Allowed && in.Allowed
	d.Remaining = min(d.Remaining, in.Remaining)
	d.RetryIn = max(d.RetryIn, in.RetryIn)
	d.ResetIn = max(d.ResetIn, in.ResetIn)
	if in.newTAT.After(d.newTAT) {
		d.newTAT = in.newTAT
	}
}

// BatchSpend attempts to deduct the costs from the provided buckets'
// capacities. If applicable, new bucket states are persisted to the underlying
// datastore before returning. Non-existent buckets will be initialized WITH the
// cost factored into the initial state. The following rules are applied to
// merge the Decisions for each Transaction into a single batch Decision:
//   - Allowed is true if all Transactions where check is true were allowed,
//   - RetryIn and ResetIn are the largest values of each across all Decisions,
//   - Remaining is the smallest value of each across all Decisions, and
//   - Decisions resulting from spend-only Transactions are never merged.
func (l *Limiter) BatchSpend(ctx context.Context, txns []Transaction) (*Decision, error) {
	batch, bucketKeys, err := prepareBatch(txns)
	if err != nil {
		return nil, err
	}
	if len(batch) == 0 {
		// All Transactions were allow-only.
		return allowedDecision, nil
	}

	// Remove cancellation from the request context so that transactions are not
	// interrupted by a client disconnect.
	ctx = context.WithoutCancel(ctx)
	tats, err := l.source.BatchGet(ctx, bucketKeys)
	if err != nil {
		return nil, err
	}

	start := l.clk.Now()
	batchDecision := newBatchDecision()
	newTATs := make(map[string]time.Time)

	for _, txn := range batch {
		tat, exists := tats[txn.bucketKey]
		if !exists {
			// First request from this client.
			tat = l.clk.Now()
		}

		d := maybeSpend(l.clk, txn.limit, tat, txn.cost)

		if txn.limit.isOverride() {
			utilization := float64(txn.limit.Burst-d.Remaining) / float64(txn.limit.Burst)
			l.overrideUsageGauge.WithLabelValues(txn.limit.name.String(), txn.limit.overrideKey).Set(utilization)
		}

		if d.Allowed && (tat != d.newTAT) && txn.spend {
			// New bucket state should be persisted.
			newTATs[txn.bucketKey] = d.newTAT
		}

		if !txn.spendOnly() {
			batchDecision.merge(d)
		}
	}

	if batchDecision.Allowed {
		err = l.source.BatchSet(ctx, newTATs)
		if err != nil {
			return nil, err
		}
		l.spendLatency.WithLabelValues("batch", Allowed).Observe(l.clk.Since(start).Seconds())
	} else {
		l.spendLatency.WithLabelValues("batch", Denied).Observe(l.clk.Since(start).Seconds())
	}
	return batchDecision.Decision, nil
}

// Refund attempts to refund all of the cost to the capacity of the specified
// bucket. The returned *Decision indicates whether the refund was successful
// and represents the current state of the bucket. The new bucket state is
// persisted to the underlying datastore, if applicable, before returning. If no
// bucket exists it will NOT be created. Spend-only Transactions are assumed to
// be refundable. Check-only Transactions are never refunded.
//
// Note: The amount refunded cannot cause the bucket to exceed its maximum
// capacity. Partial refunds are allowed and are considered successful. For
// instance, if a bucket has a maximum capacity of 10 and currently has 5
// requests remaining, a refund request of 7 will result in the bucket reaching
// its maximum capacity of 10, not 12.
func (l *Limiter) Refund(ctx context.Context, txn Transaction) (*Decision, error) {
	return l.BatchRefund(ctx, []Transaction{txn})
}

// BatchRefund attempts to refund all or some of the costs to the provided
// buckets' capacities. Non-existent buckets will NOT be initialized. The new
// bucket state is persisted to the underlying datastore, if applicable, before
// returning. Spend-only Transactions are assumed to be refundable. Check-only
// Transactions are never refunded. The following rules are applied to merge the
// Decisions for each Transaction into a single batch Decision:
//   - Allowed is true if all Transactions where check is true were allowed,
//   - RetryIn and ResetIn are the largest values of each across all Decisions,
//   - Remaining is the smallest value of each across all Decisions, and
//   - Decisions resulting from spend-only Transactions are never merged.
func (l *Limiter) BatchRefund(ctx context.Context, txns []Transaction) (*Decision, error) {
	batch, bucketKeys, err := prepareBatch(txns)
	if err != nil {
		return nil, err
	}
	if len(batch) == 0 {
		// All Transactions were allow-only.
		return allowedDecision, nil
	}

	// Remove cancellation from the request context so that transactions are not
	// interrupted by a client disconnect.
	ctx = context.WithoutCancel(ctx)
	tats, err := l.source.BatchGet(ctx, bucketKeys)
	if err != nil {
		return nil, err
	}

	batchDecision := newBatchDecision()
	newTATs := make(map[string]time.Time)

	for _, txn := range batch {
		tat, exists := tats[txn.bucketKey]
		if !exists {
			// Ignore non-existent bucket.
			continue
		}

		var cost int64
		if !txn.checkOnly() {
			cost = txn.cost
		}
		d := maybeRefund(l.clk, txn.limit, tat, cost)
		batchDecision.merge(d)
		if d.Allowed && tat != d.newTAT {
			// New bucket state should be persisted.
			newTATs[txn.bucketKey] = d.newTAT
		}
	}

	if len(newTATs) > 0 {
		err = l.source.BatchSet(ctx, newTATs)
		if err != nil {
			return nil, err
		}
	}
	return batchDecision.Decision, nil
}

// Reset resets the specified bucket to its maximum capacity. The new bucket
// state is persisted to the underlying datastore before returning.
func (l *Limiter) Reset(ctx context.Context, bucketKey string) error {
	// Remove cancellation from the request context so that transactions are not
	// interrupted by a client disconnect.
	ctx = context.WithoutCancel(ctx)
	return l.source.Delete(ctx, bucketKey)
}
