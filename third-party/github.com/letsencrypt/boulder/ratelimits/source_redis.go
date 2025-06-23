package ratelimits

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/jmhodges/clock"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
)

// Compile-time check that RedisSource implements the source interface.
var _ source = (*RedisSource)(nil)

// RedisSource is a ratelimits source backed by sharded Redis.
type RedisSource struct {
	client  *redis.Ring
	clk     clock.Clock
	latency *prometheus.HistogramVec
}

// NewRedisSource returns a new Redis backed source using the provided
// *redis.Ring client.
func NewRedisSource(client *redis.Ring, clk clock.Clock, stats prometheus.Registerer) *RedisSource {
	latency := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "ratelimits_latency",
			Help: "Histogram of Redis call latencies labeled by call=[set|get|delete|ping] and result=[success|error]",
			// Exponential buckets ranging from 0.0005s to 3s.
			Buckets: prometheus.ExponentialBucketsRange(0.0005, 3, 8),
		},
		[]string{"call", "result"},
	)
	stats.MustRegister(latency)

	return &RedisSource{
		client:  client,
		clk:     clk,
		latency: latency,
	}
}

// resultForError returns a string representing the result of the operation
// based on the provided error.
func resultForError(err error) string {
	if errors.Is(redis.Nil, err) {
		// Bucket key does not exist.
		return "notFound"
	} else if errors.Is(err, context.DeadlineExceeded) {
		// Client read or write deadline exceeded.
		return "deadlineExceeded"
	} else if errors.Is(err, context.Canceled) {
		// Caller canceled the operation.
		return "canceled"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		// Dialer timed out connecting to Redis.
		return "timeout"
	}
	var redisErr redis.Error
	if errors.Is(err, redisErr) {
		// An internal error was returned by the Redis server.
		return "redisError"
	}
	return "failed"
}

// BatchSet stores TATs at the specified bucketKeys using a pipelined Redis
// Transaction in order to reduce the number of round-trips to each Redis shard.
// An error is returned if the operation failed and nil otherwise.
func (r *RedisSource) BatchSet(ctx context.Context, buckets map[string]time.Time) error {
	start := r.clk.Now()

	pipeline := r.client.Pipeline()
	for bucketKey, tat := range buckets {
		pipeline.Set(ctx, bucketKey, tat.UTC().UnixNano(), 0)
	}
	_, err := pipeline.Exec(ctx)
	if err != nil {
		r.latency.With(prometheus.Labels{"call": "batchset", "result": resultForError(err)}).Observe(time.Since(start).Seconds())
		return err
	}

	r.latency.With(prometheus.Labels{"call": "batchset", "result": "success"}).Observe(time.Since(start).Seconds())
	return nil
}

// Get retrieves the TAT at the specified bucketKey. An error is returned if the
// operation failed and nil otherwise. If the bucketKey does not exist,
// ErrBucketNotFound is returned.
func (r *RedisSource) Get(ctx context.Context, bucketKey string) (time.Time, error) {
	start := r.clk.Now()

	tatNano, err := r.client.Get(ctx, bucketKey).Int64()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			// Bucket key does not exist.
			r.latency.With(prometheus.Labels{"call": "get", "result": "notFound"}).Observe(time.Since(start).Seconds())
			return time.Time{}, ErrBucketNotFound
		}
		r.latency.With(prometheus.Labels{"call": "get", "result": resultForError(err)}).Observe(time.Since(start).Seconds())
		return time.Time{}, err
	}

	r.latency.With(prometheus.Labels{"call": "get", "result": "success"}).Observe(time.Since(start).Seconds())
	return time.Unix(0, tatNano).UTC(), nil
}

// BatchGet retrieves the TATs at the specified bucketKeys using a pipelined
// Redis Transaction in order to reduce the number of round-trips to each Redis
// shard. An error is returned if the operation failed and nil otherwise. If a
// bucketKey does not exist, it WILL NOT be included in the returned map.
func (r *RedisSource) BatchGet(ctx context.Context, bucketKeys []string) (map[string]time.Time, error) {
	start := r.clk.Now()

	pipeline := r.client.Pipeline()
	for _, bucketKey := range bucketKeys {
		pipeline.Get(ctx, bucketKey)
	}
	results, err := pipeline.Exec(ctx)
	if err != nil {
		r.latency.With(prometheus.Labels{"call": "batchget", "result": resultForError(err)}).Observe(time.Since(start).Seconds())
		if !errors.Is(err, redis.Nil) {
			return nil, err
		}
	}

	tats := make(map[string]time.Time, len(bucketKeys))
	for i, result := range results {
		tatNano, err := result.(*redis.StringCmd).Int64()
		if err != nil {
			if errors.Is(err, redis.Nil) {
				// Bucket key does not exist.
				continue
			}
			r.latency.With(prometheus.Labels{"call": "batchget", "result": resultForError(err)}).Observe(time.Since(start).Seconds())
			return nil, err
		}
		tats[bucketKeys[i]] = time.Unix(0, tatNano).UTC()
	}

	r.latency.With(prometheus.Labels{"call": "batchget", "result": "success"}).Observe(time.Since(start).Seconds())
	return tats, nil
}

// Delete deletes the TAT at the specified bucketKey ('name:id'). It returns an
// error if the operation failed and nil otherwise. A nil return value does not
// indicate that the bucketKey existed.
func (r *RedisSource) Delete(ctx context.Context, bucketKey string) error {
	start := r.clk.Now()

	err := r.client.Del(ctx, bucketKey).Err()
	if err != nil {
		r.latency.With(prometheus.Labels{"call": "delete", "result": resultForError(err)}).Observe(time.Since(start).Seconds())
		return err
	}

	r.latency.With(prometheus.Labels{"call": "delete", "result": "success"}).Observe(time.Since(start).Seconds())
	return nil
}

// Ping checks that each shard of the *redis.Ring is reachable using the PING
// command. It returns an error if any shard is unreachable and nil otherwise.
func (r *RedisSource) Ping(ctx context.Context) error {
	start := r.clk.Now()

	err := r.client.ForEachShard(ctx, func(ctx context.Context, shard *redis.Client) error {
		return shard.Ping(ctx).Err()
	})
	if err != nil {
		r.latency.With(prometheus.Labels{"call": "ping", "result": resultForError(err)}).Observe(time.Since(start).Seconds())
		return err
	}
	r.latency.With(prometheus.Labels{"call": "ping", "result": "success"}).Observe(time.Since(start).Seconds())
	return nil
}
