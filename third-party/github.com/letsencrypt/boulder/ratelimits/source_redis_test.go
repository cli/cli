package ratelimits

import (
	"context"
	"testing"
	"time"

	"github.com/letsencrypt/boulder/cmd"
	"github.com/letsencrypt/boulder/metrics"
	"github.com/letsencrypt/boulder/test"

	"github.com/jmhodges/clock"
	"github.com/redis/go-redis/v9"
)

func newTestRedisSource(clk clock.FakeClock, addrs map[string]string) *RedisSource {
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
		Addrs:     addrs,
		Username:  "unittest-rw",
		Password:  "824968fa490f4ecec1e52d5e34916bdb60d45f8d",
		TLSConfig: tlsConfig2,
	})
	return NewRedisSource(client, clk, metrics.NoopRegisterer)
}

func newRedisTestLimiter(t *testing.T, clk clock.FakeClock) *Limiter {
	return newTestLimiter(t, newTestRedisSource(clk, map[string]string{
		"shard1": "10.33.33.4:4218",
		"shard2": "10.33.33.5:4218",
	}), clk)
}

func TestRedisSource_Ping(t *testing.T) {
	clk := clock.NewFake()
	workingSource := newTestRedisSource(clk, map[string]string{
		"shard1": "10.33.33.4:4218",
		"shard2": "10.33.33.5:4218",
	})

	err := workingSource.Ping(context.Background())
	test.AssertNotError(t, err, "Ping should not error")

	missingFirstShardSource := newTestRedisSource(clk, map[string]string{
		"shard1": "10.33.33.4:1337",
		"shard2": "10.33.33.5:4218",
	})

	err = missingFirstShardSource.Ping(context.Background())
	test.AssertError(t, err, "Ping should not error")

	missingSecondShardSource := newTestRedisSource(clk, map[string]string{
		"shard1": "10.33.33.4:4218",
		"shard2": "10.33.33.5:1337",
	})

	err = missingSecondShardSource.Ping(context.Background())
	test.AssertError(t, err, "Ping should not error")
}

func TestRedisSource_BatchSetAndGet(t *testing.T) {
	clk := clock.NewFake()
	s := newTestRedisSource(clk, map[string]string{
		"shard1": "10.33.33.4:4218",
		"shard2": "10.33.33.5:4218",
	})

	now := clk.Now()
	val1 := now.Add(time.Second)
	val2 := now.Add(time.Second * 2)
	val3 := now.Add(time.Second * 3)

	set := map[string]time.Time{
		"test1": val1,
		"test2": val2,
		"test3": val3,
	}

	err := s.BatchSet(context.Background(), set)
	test.AssertNotError(t, err, "BatchSet() should not error")

	got, err := s.BatchGet(context.Background(), []string{"test1", "test2", "test3"})
	test.AssertNotError(t, err, "BatchGet() should not error")

	for k, v := range set {
		test.Assert(t, got[k].Equal(v), "BatchGet() should return the values set by BatchSet()")
	}

	// Test that BatchGet() returns a zero time for a key that does not exist.
	got, err = s.BatchGet(context.Background(), []string{"test1", "test4", "test3"})
	test.AssertNotError(t, err, "BatchGet() should not error when a key isn't found")
	test.Assert(t, got["test4"].IsZero(), "BatchGet() should return a zero time for a key that does not exist")
}
