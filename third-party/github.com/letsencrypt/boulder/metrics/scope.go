package metrics

import "github.com/prometheus/client_golang/prometheus"

// InternetFacingBuckets are the histogram buckets that should be used when
// measuring latencies that involve traversing the public internet.
var InternetFacingBuckets = []float64{.1, .5, 1, 5, 10, 30, 45}

// noopRegisterer mocks prometheus.Registerer. It is used when we need to
// register prometheus metrics in tests where multiple registrations would
// cause a panic.
type noopRegisterer struct{}

func (np *noopRegisterer) MustRegister(_ ...prometheus.Collector) {}

func (np *noopRegisterer) Register(_ prometheus.Collector) error  { return nil }
func (np *noopRegisterer) Unregister(_ prometheus.Collector) bool { return true }

var NoopRegisterer = &noopRegisterer{}
