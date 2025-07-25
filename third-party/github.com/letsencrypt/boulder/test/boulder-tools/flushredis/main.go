package main

import (
	"context"
	"fmt"
	"os"

	"github.com/letsencrypt/boulder/cmd"
	blog "github.com/letsencrypt/boulder/log"
	"github.com/letsencrypt/boulder/metrics"
	bredis "github.com/letsencrypt/boulder/redis"

	"github.com/redis/go-redis/v9"
)

func main() {
	rc := bredis.Config{
		Username: "unittest-rw",
		TLS: cmd.TLSConfig{
			CACertFile: "test/certs/ipki/minica.pem",
			CertFile:   "test/certs/ipki/localhost/cert.pem",
			KeyFile:    "test/certs/ipki/localhost/key.pem",
		},
		Lookups: []cmd.ServiceDomain{
			{
				Service: "redisratelimits",
				Domain:  "service.consul",
			},
		},
		LookupDNSAuthority: "consul.service.consul",
	}
	rc.PasswordConfig = cmd.PasswordConfig{
		PasswordFile: "test/secrets/ratelimits_redis_password",
	}

	stats := metrics.NoopRegisterer
	log := blog.NewMock()
	ring, err := bredis.NewRingFromConfig(rc, stats, log)
	if err != nil {
		fmt.Printf("while constructing ring client: %v\n", err)
		os.Exit(1)
	}

	err = ring.ForEachShard(context.Background(), func(ctx context.Context, shard *redis.Client) error {
		cmd := shard.FlushAll(ctx)
		_, err := cmd.Result()
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		fmt.Printf("while flushing redis shards: %v\n", err)
		os.Exit(1)
	}
}
