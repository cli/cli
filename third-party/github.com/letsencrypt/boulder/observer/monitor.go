package observer

import (
	"strconv"
	"time"

	blog "github.com/letsencrypt/boulder/log"
	"github.com/letsencrypt/boulder/observer/probers"
)

type monitor struct {
	period time.Duration
	prober probers.Prober
}

// start spins off a 'Prober' goroutine on an interval of `m.period`
// with a timeout of half `m.period`
func (m monitor) start(logger blog.Logger) {
	ticker := time.NewTicker(m.period)
	timeout := m.period / 2
	for {
		go func() {
			// Attempt to probe the configured target.
			success, dur := m.prober.Probe(timeout)

			// Produce metrics to be scraped by Prometheus.
			histObservations.WithLabelValues(
				m.prober.Name(), m.prober.Kind(), strconv.FormatBool(success),
			).Observe(dur.Seconds())

			// Log the outcome of the probe attempt.
			logger.Infof(
				"kind=[%s] success=[%v] duration=[%f] name=[%s]",
				m.prober.Kind(), success, dur.Seconds(), m.prober.Name())
		}()
		<-ticker.C
	}
}
