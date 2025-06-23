package redis

import (
	"context"
	"testing"
	"time"

	"github.com/letsencrypt/boulder/cmd"
	blog "github.com/letsencrypt/boulder/log"
	"github.com/letsencrypt/boulder/metrics"
	"github.com/letsencrypt/boulder/test"

	"github.com/redis/go-redis/v9"
)

func newTestRedisRing() *redis.Ring {
	CACertFile := "../test/certs/ipki/minica.pem"
	CertFile := "../test/certs/ipki/localhost/cert.pem"
	KeyFile := "../test/certs/ipki/localhost/key.pem"
	tlsConfig := cmd.TLSConfig{
		CACertFile: CACertFile,
		CertFile:   CertFile,
		KeyFile:    KeyFile,
	}
	tlsConfig2, err := tlsConfig.Load(metrics.NoopRegisterer)
	if err != nil {
		panic(err)
	}

	client := redis.NewRing(&redis.RingOptions{
		Username:  "unittest-rw",
		Password:  "824968fa490f4ecec1e52d5e34916bdb60d45f8d",
		TLSConfig: tlsConfig2,
	})
	return client
}

func TestNewLookup(t *testing.T) {
	t.Parallel()

	logger := blog.NewMock()
	ring := newTestRedisRing()

	_, err := newLookup([]cmd.ServiceDomain{
		{
			Service: "redisratelimits",
			Domain:  "service.consul",
		},
	},
		"consul.service.consul",
		250*time.Millisecond,
		ring,
		logger,
		metrics.NoopRegisterer,
	)
	test.AssertNotError(t, err, "Expected newLookup construction to succeed")
}

func TestStart(t *testing.T) {
	t.Parallel()

	logger := blog.NewMock()
	ring := newTestRedisRing()

	lookup, err := newLookup([]cmd.ServiceDomain{
		{
			Service: "redisratelimits",
			Domain:  "service.consul",
		},
	},
		"consul.service.consul",
		250*time.Millisecond,
		ring,
		logger,
		metrics.NoopRegisterer,
	)
	test.AssertNotError(t, err, "Expected newLookup construction to succeed")

	lookup.start()
	lookup.stop()
}

func TestNewLookupWithOneFailingSRV(t *testing.T) {
	t.Parallel()

	logger := blog.NewMock()
	ring := newTestRedisRing()

	_, err := newLookup([]cmd.ServiceDomain{
		{
			Service: "doesnotexist",
			Domain:  "service.consuls",
		},
		{
			Service: "redisratelimits",
			Domain:  "service.consul",
		},
	},
		"consul.service.consul",
		250*time.Millisecond,
		ring,
		logger,
		metrics.NoopRegisterer,
	)
	test.AssertNotError(t, err, "Expected newLookup construction to succeed")
}

func TestNewLookupWithAllFailingSRV(t *testing.T) {
	t.Parallel()

	logger := blog.NewMock()
	ring := newTestRedisRing()

	_, err := newLookup([]cmd.ServiceDomain{
		{
			Service: "doesnotexist",
			Domain:  "service.consuls",
		},
		{
			Service: "doesnotexist2",
			Domain:  "service.consuls",
		},
	},
		"consul.service.consul",
		250*time.Millisecond,
		ring,
		logger,
		metrics.NoopRegisterer,
	)
	test.AssertError(t, err, "Expected newLookup construction to fail")
}

func TestUpdateNowWithAllFailingSRV(t *testing.T) {
	t.Parallel()

	logger := blog.NewMock()
	ring := newTestRedisRing()

	lookup, err := newLookup([]cmd.ServiceDomain{
		{
			Service: "redisratelimits",
			Domain:  "service.consul",
		},
	},
		"consul.service.consul",
		250*time.Millisecond,
		ring,
		logger,
		metrics.NoopRegisterer,
	)
	test.AssertNotError(t, err, "Expected newLookup construction to succeed")

	lookup.srvLookups = []cmd.ServiceDomain{
		{
			Service: "doesnotexist1",
			Domain:  "service.consul",
		},
		{
			Service: "doesnotexist2",
			Domain:  "service.consul",
		},
	}

	testCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tempErr, nonTempErr := lookup.updateNow(testCtx)
	test.AssertNotError(t, tempErr, "Expected no temporary errors")
	test.AssertError(t, nonTempErr, "Expected non-temporary errors to have occurred")
}

func TestUpdateNowWithAllFailingSRVs(t *testing.T) {
	t.Parallel()

	logger := blog.NewMock()
	ring := newTestRedisRing()

	lookup, err := newLookup([]cmd.ServiceDomain{
		{
			Service: "redisratelimits",
			Domain:  "service.consul",
		},
	},
		"consul.service.consul",
		250*time.Millisecond,
		ring,
		logger,
		metrics.NoopRegisterer,
	)
	test.AssertNotError(t, err, "Expected newLookup construction to succeed")

	// Replace the dnsAuthority with a non-existent DNS server, this will cause
	// a timeout error, which is technically a temporary error, but will
	// eventually result in a non-temporary error when no shards are resolved.
	lookup.dnsAuthority = "consuls.services.consuls:53"

	testCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tempErr, nonTempErr := lookup.updateNow(testCtx)
	test.AssertError(t, tempErr, "Expected temporary errors")
	test.AssertError(t, nonTempErr, "Expected a non-temporary error")
	test.AssertErrorIs(t, nonTempErr, ErrNoShardsResolved)
}

func TestUpdateNowWithOneFailingSRV(t *testing.T) {
	t.Parallel()

	logger := blog.NewMock()
	ring := newTestRedisRing()

	lookup, err := newLookup([]cmd.ServiceDomain{
		{
			Service: "doesnotexist",
			Domain:  "service.consuls",
		},
		{
			Service: "redisratelimits",
			Domain:  "service.consul",
		},
	},
		"consul.service.consul",
		250*time.Millisecond,
		ring,
		logger,
		metrics.NoopRegisterer,
	)
	test.AssertNotError(t, err, "Expected newLookup construction to succeed")

	// The Consul service entry for 'redisratelimits' is configured to return
	// two SRV targets. We should only have two shards in the ring.
	test.Assert(t, ring.Len() == 2, "Expected 2 shards in the ring")

	testCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Ensure we can reach both shards using the PING command.
	err = ring.ForEachShard(testCtx, func(ctx context.Context, shard *redis.Client) error {
		return shard.Ping(ctx).Err()
	})
	test.AssertNotError(t, err, "Expected PING to succeed for both shards")

	// Drop both Shards from the ring.
	ring.SetAddrs(map[string]string{})
	test.Assert(t, ring.Len() == 0, "Expected 0 shards in the ring")

	// Force a lookup to occur.
	tempErr, nonTempErr := lookup.updateNow(testCtx)
	test.AssertNotError(t, tempErr, "Expected no temporary errors")
	test.AssertNotError(t, nonTempErr, "Expected no non-temporary errors")

	// The ring should now have two shards again.
	test.Assert(t, ring.Len() == 2, "Expected 2 shards in the ring")
}
