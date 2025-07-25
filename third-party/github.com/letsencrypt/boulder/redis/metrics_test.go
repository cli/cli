package redis

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"

	"github.com/letsencrypt/boulder/metrics"
)

type mockPoolStatGetter struct{}

var _ poolStatGetter = mockPoolStatGetter{}

func (mockPoolStatGetter) PoolStats() *redis.PoolStats {
	return &redis.PoolStats{
		Hits:       13,
		Misses:     7,
		Timeouts:   4,
		TotalConns: 1000,
		IdleConns:  500,
		StaleConns: 10,
	}
}

func TestMetrics(t *testing.T) {
	mets := newClientMetricsCollector(mockPoolStatGetter{},
		prometheus.Labels{
			"foo": "bar",
		})
	// Check that it has the correct type to satisfy MustRegister
	metrics.NoopRegisterer.MustRegister(mets)

	expectedMetrics := 6
	outChan := make(chan prometheus.Metric, expectedMetrics)
	mets.Collect(outChan)

	results := make(map[string]bool)
	for range expectedMetrics {
		metric := <-outChan
		t.Log(metric.Desc().String())
		results[metric.Desc().String()] = true
	}

	expected := strings.Split(
		`Desc{fqName: "redis_connection_pool_lookups", help: "Number of lookups for a connection in the pool, labeled by hit/miss", constLabels: {foo="bar"}, variableLabels: {result}}
Desc{fqName: "redis_connection_pool_lookups", help: "Number of lookups for a connection in the pool, labeled by hit/miss", constLabels: {foo="bar"}, variableLabels: {result}}
Desc{fqName: "redis_connection_pool_lookups", help: "Number of lookups for a connection in the pool, labeled by hit/miss", constLabels: {foo="bar"}, variableLabels: {result}}
Desc{fqName: "redis_connection_pool_total_conns", help: "Number of total connections in the pool.", constLabels: {foo="bar"}, variableLabels: {}}
Desc{fqName: "redis_connection_pool_idle_conns", help: "Number of idle connections in the pool.", constLabels: {foo="bar"}, variableLabels: {}}
Desc{fqName: "redis_connection_pool_stale_conns", help: "Number of stale connections removed from the pool.", constLabels: {foo="bar"}, variableLabels: {}}`,
		"\n")

	for _, e := range expected {
		if !results[e] {
			t.Errorf("expected metrics to contain %q, but they didn't", e)
		}
	}

	if len(results) > len(expected) {
		t.Errorf("expected metrics to contain %d entries, but they contained %d",
			len(expected), len(results))
	}
}

func TestMustRegisterClientMetricsCollector(t *testing.T) {
	client := mockPoolStatGetter{}
	stats := prometheus.NewRegistry()
	// First registration should succeed.
	MustRegisterClientMetricsCollector(client, stats, map[string]string{"foo": "bar"}, "baz")
	// Duplicate registration should succeed.
	MustRegisterClientMetricsCollector(client, stats, map[string]string{"foo": "bar"}, "baz")
	// Registration with different label values should succeed.
	MustRegisterClientMetricsCollector(client, stats, map[string]string{"f00": "b4r"}, "b4z")
}
