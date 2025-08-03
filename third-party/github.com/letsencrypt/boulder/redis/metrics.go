package redis

import (
	"errors"
	"slices"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
)

// An interface satisfied by *redis.ClusterClient and also by a mock in our tests.
type poolStatGetter interface {
	PoolStats() *redis.PoolStats
}

var _ poolStatGetter = (*redis.ClusterClient)(nil)

type metricsCollector struct {
	statGetter poolStatGetter

	// Stats accessible from the go-redis connector:
	// https://pkg.go.dev/github.com/go-redis/redis@v6.15.9+incompatible/internal/pool#Stats
	lookups    *prometheus.Desc
	totalConns *prometheus.Desc
	idleConns  *prometheus.Desc
	staleConns *prometheus.Desc
}

// Describe is implemented with DescribeByCollect. That's possible because the
// Collect method will always return the same metrics with the same descriptors.
func (dbc metricsCollector) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(dbc, ch)
}

// Collect first triggers the Redis ClusterClient's PoolStats function.
// Then it creates constant metrics for each Stats value on the fly based
// on the returned data.
//
// Note that Collect could be called concurrently, so we depend on PoolStats()
// to be concurrency-safe.
func (dbc metricsCollector) Collect(ch chan<- prometheus.Metric) {
	writeGauge := func(stat *prometheus.Desc, val uint32, labelValues ...string) {
		ch <- prometheus.MustNewConstMetric(stat, prometheus.GaugeValue, float64(val), labelValues...)
	}

	stats := dbc.statGetter.PoolStats()
	writeGauge(dbc.lookups, stats.Hits, "hit")
	writeGauge(dbc.lookups, stats.Misses, "miss")
	writeGauge(dbc.lookups, stats.Timeouts, "timeout")
	writeGauge(dbc.totalConns, stats.TotalConns)
	writeGauge(dbc.idleConns, stats.IdleConns)
	writeGauge(dbc.staleConns, stats.StaleConns)
}

// newClientMetricsCollector is broken out for testing purposes.
func newClientMetricsCollector(statGetter poolStatGetter, labels prometheus.Labels) metricsCollector {
	return metricsCollector{
		statGetter: statGetter,
		lookups: prometheus.NewDesc(
			"redis_connection_pool_lookups",
			"Number of lookups for a connection in the pool, labeled by hit/miss",
			[]string{"result"}, labels),
		totalConns: prometheus.NewDesc(
			"redis_connection_pool_total_conns",
			"Number of total connections in the pool.",
			nil, labels),
		idleConns: prometheus.NewDesc(
			"redis_connection_pool_idle_conns",
			"Number of idle connections in the pool.",
			nil, labels),
		staleConns: prometheus.NewDesc(
			"redis_connection_pool_stale_conns",
			"Number of stale connections removed from the pool.",
			nil, labels),
	}
}

// MustRegisterClientMetricsCollector registers a metrics collector for the
// given Redis client with the provided prometheus.Registerer. The collector
// will report metrics labelled by the provided addresses and username. If the
// collector is already registered, this function is a no-op.
func MustRegisterClientMetricsCollector(client poolStatGetter, stats prometheus.Registerer, addrs map[string]string, user string) {
	var labelAddrs []string
	for addr := range addrs {
		labelAddrs = append(labelAddrs, addr)
	}
	// Keep the list of addresses sorted for consistency.
	slices.Sort(labelAddrs)
	labels := prometheus.Labels{
		"addresses": strings.Join(labelAddrs, ", "),
		"user":      user,
	}
	err := stats.Register(newClientMetricsCollector(client, labels))
	if err != nil {
		are := prometheus.AlreadyRegisteredError{}
		if errors.As(err, &are) {
			// The collector is already registered using the same labels.
			return
		}
		panic(err)
	}
}
