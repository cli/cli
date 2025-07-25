package rocsp_config

import (
	"bytes"
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
	"fmt"
	"strings"

	"github.com/jmhodges/clock"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/ocsp"

	"github.com/letsencrypt/boulder/cmd"
	"github.com/letsencrypt/boulder/config"
	"github.com/letsencrypt/boulder/issuance"
	bredis "github.com/letsencrypt/boulder/redis"
	"github.com/letsencrypt/boulder/rocsp"
)

// RedisConfig contains the configuration needed to act as a Redis client.
//
// TODO(#7081): Deprecate this in favor of bredis.Config once we can support SRV
// lookups in rocsp.
type RedisConfig struct {
	// PasswordFile is a file containing the password for the Redis user.
	cmd.PasswordConfig
	// TLS contains the configuration to speak TLS with Redis.
	TLS cmd.TLSConfig
	// Username is a Redis username.
	Username string `validate:"required"`
	// ShardAddrs is a map of shard names to IP address:port pairs. The go-redis
	// `Ring` client will shard reads and writes across the provided Redis
	// Servers based on a consistent hashing algorithm.
	ShardAddrs map[string]string `validate:"min=1,dive,hostname_port"`
	// Timeout is a per-request timeout applied to all Redis requests.
	Timeout config.Duration `validate:"-"`

	// Enables read-only commands on replicas.
	ReadOnly bool
	// Allows routing read-only commands to the closest primary or replica.
	// It automatically enables ReadOnly.
	RouteByLatency bool
	// Allows routing read-only commands to a random primary or replica.
	// It automatically enables ReadOnly.
	RouteRandomly bool

	// PoolFIFO uses FIFO mode for each node connection pool GET/PUT (default LIFO).
	PoolFIFO bool

	// Maximum number of retries before giving up.
	// Default is to not retry failed commands.
	MaxRetries int `validate:"min=0"`
	// Minimum backoff between each retry.
	// Default is 8 milliseconds; -1 disables backoff.
	MinRetryBackoff config.Duration `validate:"-"`
	// Maximum backoff between each retry.
	// Default is 512 milliseconds; -1 disables backoff.
	MaxRetryBackoff config.Duration `validate:"-"`

	// Dial timeout for establishing new connections.
	// Default is 5 seconds.
	DialTimeout config.Duration `validate:"-"`
	// Timeout for socket reads. If reached, commands will fail
	// with a timeout instead of blocking. Use value -1 for no timeout and 0 for default.
	// Default is 3 seconds.
	ReadTimeout config.Duration `validate:"-"`
	// Timeout for socket writes. If reached, commands will fail
	// with a timeout instead of blocking.
	// Default is ReadTimeout.
	WriteTimeout config.Duration `validate:"-"`

	// Maximum number of socket connections.
	// Default is 5 connections per every CPU as reported by runtime.NumCPU.
	// If this is set to an explicit value, that's not multiplied by NumCPU.
	// PoolSize applies per cluster node and not for the whole cluster.
	// https://pkg.go.dev/github.com/go-redis/redis#ClusterOptions
	PoolSize int `validate:"min=0"`
	// Minimum number of idle connections which is useful when establishing
	// new connection is slow.
	MinIdleConns int `validate:"min=0"`
	// Connection age at which client retires (closes) the connection.
	// Default is to not close aged connections.
	MaxConnAge config.Duration `validate:"-"`
	// Amount of time client waits for connection if all connections
	// are busy before returning an error.
	// Default is ReadTimeout + 1 second.
	PoolTimeout config.Duration `validate:"-"`
	// Amount of time after which client closes idle connections.
	// Should be less than server's timeout.
	// Default is 5 minutes. -1 disables idle timeout check.
	IdleTimeout config.Duration `validate:"-"`
	// Frequency of idle checks made by idle connections reaper.
	// Default is 1 minute. -1 disables idle connections reaper,
	// but idle connections are still discarded by the client
	// if IdleTimeout is set.
	// Deprecated: This field has been deprecated and will be removed.
	IdleCheckFrequency config.Duration `validate:"-"`
}

// MakeClient produces a read-write ROCSP client from a config.
func MakeClient(c *RedisConfig, clk clock.Clock, stats prometheus.Registerer) (*rocsp.RWClient, error) {
	password, err := c.PasswordConfig.Pass()
	if err != nil {
		return nil, fmt.Errorf("loading password: %w", err)
	}

	tlsConfig, err := c.TLS.Load(stats)
	if err != nil {
		return nil, fmt.Errorf("loading TLS config: %w", err)
	}

	rdb := redis.NewRing(&redis.RingOptions{
		Addrs:     c.ShardAddrs,
		Username:  c.Username,
		Password:  password,
		TLSConfig: tlsConfig,

		MaxRetries:      c.MaxRetries,
		MinRetryBackoff: c.MinRetryBackoff.Duration,
		MaxRetryBackoff: c.MaxRetryBackoff.Duration,
		DialTimeout:     c.DialTimeout.Duration,
		ReadTimeout:     c.ReadTimeout.Duration,
		WriteTimeout:    c.WriteTimeout.Duration,

		PoolSize:        c.PoolSize,
		MinIdleConns:    c.MinIdleConns,
		ConnMaxLifetime: c.MaxConnAge.Duration,
		PoolTimeout:     c.PoolTimeout.Duration,
		ConnMaxIdleTime: c.IdleTimeout.Duration,
	})
	return rocsp.NewWritingClient(rdb, c.Timeout.Duration, clk, stats), nil
}

// MakeReadClient produces a read-only ROCSP client from a config.
func MakeReadClient(c *RedisConfig, clk clock.Clock, stats prometheus.Registerer) (*rocsp.ROClient, error) {
	if len(c.ShardAddrs) == 0 {
		return nil, errors.New("redis config's 'shardAddrs' field was empty")
	}

	password, err := c.PasswordConfig.Pass()
	if err != nil {
		return nil, fmt.Errorf("loading password: %w", err)
	}

	tlsConfig, err := c.TLS.Load(stats)
	if err != nil {
		return nil, fmt.Errorf("loading TLS config: %w", err)
	}

	rdb := redis.NewRing(&redis.RingOptions{
		Addrs:     c.ShardAddrs,
		Username:  c.Username,
		Password:  password,
		TLSConfig: tlsConfig,

		PoolFIFO: c.PoolFIFO,

		MaxRetries:      c.MaxRetries,
		MinRetryBackoff: c.MinRetryBackoff.Duration,
		MaxRetryBackoff: c.MaxRetryBackoff.Duration,
		DialTimeout:     c.DialTimeout.Duration,
		ReadTimeout:     c.ReadTimeout.Duration,

		PoolSize:        c.PoolSize,
		MinIdleConns:    c.MinIdleConns,
		ConnMaxLifetime: c.MaxConnAge.Duration,
		PoolTimeout:     c.PoolTimeout.Duration,
		ConnMaxIdleTime: c.IdleTimeout.Duration,
	})
	bredis.MustRegisterClientMetricsCollector(rdb, stats, rdb.Options().Addrs, rdb.Options().Username)
	return rocsp.NewReadingClient(rdb, c.Timeout.Duration, clk, stats), nil
}

// A ShortIDIssuer combines an issuance.Certificate with some fields necessary
// to process OCSP responses: the subject name and the shortID.
type ShortIDIssuer struct {
	*issuance.Certificate
	subject pkix.RDNSequence
	shortID byte
}

// LoadIssuers takes a map where the keys are filenames and the values are the
// corresponding short issuer ID. It loads issuer certificates from the given
// files and produces a []ShortIDIssuer.
func LoadIssuers(input map[string]int) ([]ShortIDIssuer, error) {
	var issuers []ShortIDIssuer
	for issuerFile, shortID := range input {
		if shortID > 255 || shortID < 0 {
			return nil, fmt.Errorf("invalid shortID %d (must be byte)", shortID)
		}
		cert, err := issuance.LoadCertificate(issuerFile)
		if err != nil {
			return nil, fmt.Errorf("reading issuer: %w", err)
		}
		var subject pkix.RDNSequence
		_, err = asn1.Unmarshal(cert.Certificate.RawSubject, &subject)
		if err != nil {
			return nil, fmt.Errorf("parsing issuer.RawSubject: %w", err)
		}
		shortID := byte(shortID)
		for _, issuer := range issuers {
			if issuer.shortID == shortID {
				return nil, fmt.Errorf("duplicate shortID '%d' in (for %q and %q) in config file", shortID, issuer.subject, subject)
			}
			if !issuer.IsCA {
				return nil, fmt.Errorf("certificate for %q is not a CA certificate", subject)
			}
		}
		issuers = append(issuers, ShortIDIssuer{
			Certificate: cert,
			subject:     subject,
			shortID:     shortID,
		})
	}
	return issuers, nil
}

// ShortID returns the short ID of an issuer. The short ID is a single byte that
// is unique for that issuer.
func (si *ShortIDIssuer) ShortID() byte {
	return si.shortID
}

// FindIssuerByID returns the issuer that matches the given IssuerNameID.
func FindIssuerByID(longID int64, issuers []ShortIDIssuer) (*ShortIDIssuer, error) {
	for _, iss := range issuers {
		if iss.NameID() == issuance.NameID(longID) {
			return &iss, nil
		}
	}
	return nil, fmt.Errorf("no issuer found for an ID in certificateStatus: %d", longID)
}

// FindIssuerByName returns the issuer with a Subject matching the *ocsp.Response.
func FindIssuerByName(resp *ocsp.Response, issuers []ShortIDIssuer) (*ShortIDIssuer, error) {
	var responder pkix.RDNSequence
	_, err := asn1.Unmarshal(resp.RawResponderName, &responder)
	if err != nil {
		return nil, fmt.Errorf("parsing resp.RawResponderName: %w", err)
	}
	var responders strings.Builder
	for _, issuer := range issuers {
		fmt.Fprintf(&responders, "%s\n", issuer.subject)
		if bytes.Equal(issuer.RawSubject, resp.RawResponderName) {
			return &issuer, nil
		}
	}
	return nil, fmt.Errorf("no issuer found matching OCSP response for %s. Available issuers:\n%s\n", responder, responders.String())
}
