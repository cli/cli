package measured_http

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/jmhodges/clock"
	"github.com/prometheus/client_golang/prometheus"
	io_prometheus_client "github.com/prometheus/client_model/go"
)

type sleepyHandler struct {
	clk clock.FakeClock
}

func (h sleepyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.clk.Sleep(999 * time.Second)
	w.WriteHeader(302)
}

func collect(m prometheus.Collector) *io_prometheus_client.Metric {
	ch := make(chan prometheus.Metric, 10)
	m.Collect(ch)
	result := <-ch
	var iom = new(io_prometheus_client.Metric)
	_ = result.Write(iom)
	return iom
}

func TestMeasuring(t *testing.T) {
	clk := clock.NewFake()

	// Create a local histogram stat with the same labels as the real one, but
	// don't register it; we will collect its data here in the test to verify it.
	stat := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "fake",
			Help: "fake",
		},
		[]string{"endpoint", "method", "code"})

	mux := http.NewServeMux()
	mux.Handle("/foo", sleepyHandler{clk})
	mh := MeasuredHandler{
		serveMux: mux,
		clk:      clk,
		stat:     stat,
	}
	mh.ServeHTTP(httptest.NewRecorder(), &http.Request{
		URL:    &url.URL{Path: "/foo"},
		Method: "GET",
	})
	iom := collect(stat)

	hist := iom.Histogram
	if *hist.SampleCount != 1 {
		t.Errorf("SampleCount = %d (expected 1)", *hist.SampleCount)
	}
	if *hist.SampleSum != 999 {
		t.Errorf("SampleSum = %g (expected 999)", *hist.SampleSum)
	}

	expectedLabels := map[string]string{
		"endpoint": "/foo",
		"method":   "GET",
		"code":     "302",
	}
	for _, labelPair := range iom.Label {
		if expectedLabels[*labelPair.Name] == "" {
			t.Errorf("Unexpected label %s", *labelPair.Name)
		} else if expectedLabels[*labelPair.Name] != *labelPair.Value {
			t.Errorf("labels[%q] = %q (expected %q)", *labelPair.Name, *labelPair.Value,
				expectedLabels[*labelPair.Name])
		}
		delete(expectedLabels, *labelPair.Name)
	}
	if len(expectedLabels) != 0 {
		t.Errorf("Some labels were expected, but not observed: %v", expectedLabels)
	}
}

// Make an HTTP request with an unknown method and ensure we use the appropriate
// label value.
func TestUnknownMethod(t *testing.T) {
	clk := clock.NewFake()

	// Create a local histogram stat with the same labels as the real one, but
	// don't register it; we will collect its data here in the test to verify it.
	stat := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "fake",
			Help: "fake",
		},
		[]string{"endpoint", "method", "code"})

	mux := http.NewServeMux()
	mux.Handle("/foo", sleepyHandler{clk})
	mh := MeasuredHandler{
		serveMux: mux,
		clk:      clk,
		stat:     stat,
	}
	mh.ServeHTTP(httptest.NewRecorder(), &http.Request{
		URL:    &url.URL{Path: "/foo"},
		Method: "POKE",
	})
	iom := collect(stat)

	expectedLabels := map[string]string{
		"endpoint": "/foo",
		"method":   "unknown",
		"code":     "302",
	}
	for _, labelPair := range iom.Label {
		if expectedLabels[*labelPair.Name] == "" {
			t.Errorf("Unexpected label %s", *labelPair.Name)
		} else if expectedLabels[*labelPair.Name] != *labelPair.Value {
			t.Errorf("labels[%q] = %q (expected %q)", *labelPair.Name, *labelPair.Value,
				expectedLabels[*labelPair.Name])
		}
		delete(expectedLabels, *labelPair.Name)
	}
	if len(expectedLabels) != 0 {
		t.Errorf("Some labels were expected, but not observed: %v", expectedLabels)
	}
}

func TestWrite(t *testing.T) {
	clk := clock.NewFake()

	// Create a local histogram stat with the same labels as the real one, but
	// don't register it; we will collect its data here in the test to verify it.
	stat := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "fake",
			Help: "fake",
		},
		[]string{"endpoint", "method", "code"})

	mux := http.NewServeMux()
	mux.HandleFunc("/foo", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte{})
	})
	mh := MeasuredHandler{
		serveMux: mux,
		clk:      clk,
		stat:     stat,
	}
	mh.ServeHTTP(httptest.NewRecorder(), &http.Request{
		URL:    &url.URL{Path: "/foo"},
		Method: "GET",
	})
	iom := collect(stat)

	stat = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "fake",
			Help: "fake",
		},
		[]string{"endpoint", "method", "code"})
	mh.stat = stat
	expectedLabels := map[string]string{
		"endpoint": "/foo",
		"method":   "GET",
		"code":     "200",
	}
	for _, labelPair := range iom.Label {
		if expectedLabels[*labelPair.Name] == "" {
			t.Errorf("Unexpected label %s", *labelPair.Name)
		} else if expectedLabels[*labelPair.Name] != *labelPair.Value {
			t.Errorf("labels[%q] = %q (expected %q)", *labelPair.Name, *labelPair.Value,
				expectedLabels[*labelPair.Name])
		}
		delete(expectedLabels, *labelPair.Name)
	}
	if len(expectedLabels) != 0 {
		t.Errorf("Some labels were expected, but not observed: %v", expectedLabels)
	}

	mux.HandleFunc("/bar", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(202)
		w.Write([]byte{})
	})
	mh.ServeHTTP(httptest.NewRecorder(), &http.Request{
		URL:    &url.URL{Path: "/bar"},
		Method: "GET",
	})
	iom = collect(stat)

	expectedLabels = map[string]string{
		"endpoint": "/bar",
		"method":   "GET",
		"code":     "202",
	}
	for _, labelPair := range iom.Label {
		if expectedLabels[*labelPair.Name] == "" {
			t.Errorf("Unexpected label %s", *labelPair.Name)
		} else if expectedLabels[*labelPair.Name] != *labelPair.Value {
			t.Errorf("labels[%q] = %q (expected %q)", *labelPair.Name, *labelPair.Value,
				expectedLabels[*labelPair.Name])
		}
		delete(expectedLabels, *labelPair.Name)
	}
	if len(expectedLabels) != 0 {
		t.Errorf("Some labels were expected, but not observed: %v", expectedLabels)
	}
}
