package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/letsencrypt/boulder/test/ocsp/helper"
	prom "github.com/prometheus/client_golang/prometheus"
	promhttp "github.com/prometheus/client_golang/prometheus/promhttp"
)

var listenAddress = flag.String("listen", ":8080", "Port to listen on")
var interval = flag.String("interval", "1m", "Time to sleep between fetches")

var (
	response_count = prom.NewCounterVec(prom.CounterOpts{
		Name: "responses",
		Help: "completed responses",
	}, nil)
	errors_count = prom.NewCounterVec(prom.CounterOpts{
		Name: "errors",
		Help: "errored responses",
	}, nil)
	request_time_seconds_hist = prom.NewHistogram(prom.HistogramOpts{
		Name: "request_time_seconds",
		Help: "time a request takes",
	})
	request_time_seconds_summary = prom.NewSummary(prom.SummaryOpts{
		Name: "request_time_seconds_summary",
		Help: "time a request takes",
	})
	response_age_seconds = prom.NewHistogram(prom.HistogramOpts{
		Name: "response_age_seconds",
		Help: "how old OCSP responses were",
		Buckets: []float64{24 * time.Hour.Seconds(), 48 * time.Hour.Seconds(),
			72 * time.Hour.Seconds(), 96 * time.Hour.Seconds(), 120 * time.Hour.Seconds()},
	})
	response_age_seconds_summary = prom.NewSummary(prom.SummaryOpts{
		Name:       "response_age_seconds_summary",
		Help:       "how old OCSP responses were",
		Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001, 1: 0.0001},
	})
)

func init() {
	prom.MustRegister(response_count)
	prom.MustRegister(request_time_seconds_hist)
	prom.MustRegister(request_time_seconds_summary)
	prom.MustRegister(response_age_seconds)
	prom.MustRegister(response_age_seconds_summary)
}

func do(f string, config helper.Config) {
	start := time.Now()
	resp, err := helper.ReqFile(f, config)
	latency := time.Since(start)
	if err != nil {
		errors_count.With(prom.Labels{}).Inc()
		fmt.Fprintf(os.Stderr, "error for %s: %s\n", f, err)
	}
	request_time_seconds_hist.Observe(latency.Seconds())
	response_count.With(prom.Labels{}).Inc()
	request_time_seconds_summary.Observe(latency.Seconds())
	if resp != nil {
		response_age_seconds.Observe(time.Since(resp.ThisUpdate).Seconds())
		response_age_seconds_summary.Observe(time.Since(resp.ThisUpdate).Seconds())
	}
}

func main() {
	helper.RegisterFlags()
	flag.Parse()

	config, err := helper.ConfigFromFlags()
	if err != nil {
		log.Fatal(err)
	}
	sleepTime, err := time.ParseDuration(*interval)
	if err != nil {
		log.Fatal(err)
	}
	http.Handle("/metrics", promhttp.Handler())
	go func() {
		// The gosec linter complains that timeouts cannot be set here. That's fine,
		// because this is test-only code.
		////nolint:gosec
		err := http.ListenAndServe(*listenAddress, nil)
		if err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
	for {
		for _, pattern := range flag.Args() {
			// Note: re-glob this pattern on each run, in case new certificates have
			// been added. This makes it easy to keep the list of certificates to be
			// checked fresh.
			files, err := filepath.Glob(pattern)
			if err != nil {
				log.Fatal(err)
			}
			// Loop through the available files (potentially hundreds or thousands),
			// requesting one response per `sleepTime`
			for _, f := range files {
				do(f, config)
				time.Sleep(sleepTime)
			}
		}
	}
}
