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
var _ Source = (*RedisSource)(nil)

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

var errMixedSuccess = errors.New("some keys not found")

// resultForError returns a string representing the result of the operation
// based on the provided error.
func resultForError(err error) string {
	if errors.Is(errMixedSuccess, err) {
		// Indicates that some of the keys in a batchset operation were not found.
		return "mixedSuccess"
	} else if errors.Is(redis.Nil, err) {
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

func (r *RedisSource) observeLatency(call string, latency time.Duration, err error) {
	result := "success"
	if err != nil {
		result = resultForError(err)
	}
	r.latency.With(prometheus.Labels{"call": call, "result": result}).Observe(latency.Seconds())
}

// BatchSet stores TATs at the specified bucketKeys using a pipelined Redis
// Transaction in order to reduce the number of round-trips to each Redis shard.
func (r *RedisSource) BatchSet(ctx context.Context, buckets map[string]time.Time) error {
	start := r.clk.Now()

	pipeline := r.client.Pipeline()
	for bucketKey, tat := range buckets {
		// Set a TTL of TAT + 10 minutes to account for clock skew.
		ttl := tat.UTC().Sub(r.clk.Now()) + 10*time.Minute
		pipeline.Set(ctx, bucketKey, tat.UTC().UnixNano(), ttl)
	}
	_, err := pipeline.Exec(ctx)
	if err != nil {
		r.observeLatency("batchset", r.clk.Since(start), err)
		return err
	}

	totalLatency := r.clk.Since(start)

	r.observeLatency("batchset", totalLatency, nil)
	return nil
}

// BatchSetNotExisting attempts to set TATs for the specified bucketKeys if they
// do not already exist. Returns a map indicating which keys already existed.
func (r *RedisSource) BatchSetNotExisting(ctx context.Context, buckets map[string]time.Time) (map[string]bool, error) {
	start := r.clk.Now()

	pipeline := r.client.Pipeline()
	cmds := make(map[string]*redis.BoolCmd, len(buckets))
	for bucketKey, tat := range buckets {
		// Set a TTL of TAT + 10 minutes to account for clock skew.
		ttl := tat.UTC().Sub(r.clk.Now()) + 10*time.Minute
		cmds[bucketKey] = pipeline.SetNX(ctx, bucketKey, tat.UTC().UnixNano(), ttl)
	}
	_, err := pipeline.Exec(ctx)
	if err != nil {
		r.observeLatency("batchsetnotexisting", r.clk.Since(start), err)
		return nil, err
	}

	alreadyExists := make(map[string]bool, len(buckets))
	totalLatency := r.clk.Since(start)
	for bucketKey, cmd := range cmds {
		success, err := cmd.Result()
		if err != nil {
			return nil, err
		}
		if !success {
			alreadyExists[bucketKey] = true
		}
	}

	r.observeLatency("batchsetnotexisting", totalLatency, nil)
	return alreadyExists, nil
}

// BatchIncrement updates TATs for the specified bucketKeys using a pipelined
// Redis Transaction in order to reduce the number of round-trips to each Redis
// shard.
func (r *RedisSource) BatchIncrement(ctx context.Context, buckets map[string]increment) error {
	start := r.clk.Now()

	pipeline := r.client.Pipeline()
	for bucketKey, incr := range buckets {
		pipeline.IncrBy(ctx, bucketKey, incr.cost.Nanoseconds())
		pipeline.Expire(ctx, bucketKey, incr.ttl)
	}
	_, err := pipeline.Exec(ctx)
	if err != nil {
		r.observeLatency("batchincrby", r.clk.Since(start), err)
		return err
	}

	totalLatency := r.clk.Since(start)
	r.observeLatency("batchincrby", totalLatency, nil)
	return nil
}

// Get retrieves the TAT at the specified bucketKey. If the bucketKey does not
// exist, ErrBucketNotFound is returned.
func (r *RedisSource) Get(ctx context.Context, bucketKey string) (time.Time, error) {
	start := r.clk.Now()

	tatNano, err := r.client.Get(ctx, bucketKey).Int64()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			// Bucket key does not exist.
			r.observeLatency("get", r.clk.Since(start), err)
			return time.Time{}, ErrBucketNotFound
		}
		// An error occurred while retrieving the TAT.
		r.observeLatency("get", r.clk.Since(start), err)
		return time.Time{}, err
	}

	r.observeLatency("get", r.clk.Since(start), nil)
	return time.Unix(0, tatNano).UTC(), nil
}

// BatchGet retrieves the TATs at the specified bucketKeys using a pipelined
// Redis Transaction in order to reduce the number of round-trips to each Redis
// shard. If a bucketKey does not exist, it WILL NOT be included in the returned
// map.
func (r *RedisSource) BatchGet(ctx context.Context, bucketKeys []string) (map[string]time.Time, error) {
	start := r.clk.Now()

	pipeline := r.client.Pipeline()
	for _, bucketKey := range bucketKeys {
		pipeline.Get(ctx, bucketKey)
	}
	results, err := pipeline.Exec(ctx)
	if err != nil && !errors.Is(err, redis.Nil) {
		r.observeLatency("batchget", r.clk.Since(start), err)
		return nil, err
	}

	totalLatency := r.clk.Since(start)

	tats := make(map[string]time.Time, len(bucketKeys))
	notFoundCount := 0
	for i, result := range results {
		tatNano, err := result.(*redis.StringCmd).Int64()
		if err != nil {
			if !errors.Is(err, redis.Nil) {
				// This should never happen as any errors should have been
				// caught after the pipeline.Exec() call.
				r.observeLatency("batchget", r.clk.Since(start), err)
				return nil, err
			}
			notFoundCount++
			continue
		}
		tats[bucketKeys[i]] = time.Unix(0, tatNano).UTC()
	}

	var batchErr error
	if notFoundCount < len(results) {
		// Some keys were not found.
		batchErr = errMixedSuccess
	} else if notFoundCount == len(results) {
		// All keys were not found.
		batchErr = redis.Nil
	}

	r.observeLatency("batchget", totalLatency, batchErr)
	return tats, nil
}

// Delete deletes the TAT at the specified bucketKey ('name:id'). A nil return
// value does not indicate that the bucketKey existed.
func (r *RedisSource) Delete(ctx context.Context, bucketKey string) error {
	start := r.clk.Now()

	err := r.client.Del(ctx, bucketKey).Err()
	if err != nil {
		r.observeLatency("delete", r.clk.Since(start), err)
		return err
	}

	r.observeLatency("delete", r.clk.Since(start), nil)
	return nil
}

// Ping checks that each shard of the *redis.Ring is reachable using the PING
// command.
func (r *RedisSource) Ping(ctx context.Context) error {
	start := r.clk.Now()

	err := r.client.ForEachShard(ctx, func(ctx context.Context, shard *redis.Client) error {
		return shard.Ping(ctx).Err()
	})
	if err != nil {
		r.observeLatency("ping", r.clk.Since(start), err)
		return err
	}

	r.observeLatency("ping", r.clk.Since(start), nil)
	return nil
}
