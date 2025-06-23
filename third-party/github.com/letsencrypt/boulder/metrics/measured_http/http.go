package measured_http

import (
	"net/http"
	"strconv"

	"github.com/jmhodges/clock"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// responseWriterWithStatus satisfies http.ResponseWriter, but keeps track of the
// status code for gathering stats.
type responseWriterWithStatus struct {
	http.ResponseWriter
	code int
}

// WriteHeader stores a status code for generating stats.
func (r *responseWriterWithStatus) WriteHeader(code int) {
	r.code = code
	r.ResponseWriter.WriteHeader(code)
}

// Write writes the body and sets the status code to 200 if a status code
// has not already been set.
func (r *responseWriterWithStatus) Write(body []byte) (int, error) {
	if r.code == 0 {
		r.code = http.StatusOK
	}
	return r.ResponseWriter.Write(body)
}

// serveMux is a partial interface wrapper for the method http.ServeMux
// exposes that we use. This is needed so that we can replace the default
// http.ServeMux in ocsp-responder where we don't want to use its path
// canonicalization.
type serveMux interface {
	Handler(*http.Request) (http.Handler, string)
}

// MeasuredHandler wraps an http.Handler and records prometheus stats
type MeasuredHandler struct {
	serveMux
	clk clock.Clock
	// Normally this is always responseTime, but we override it for testing.
	stat *prometheus.HistogramVec
}

func New(m serveMux, clk clock.Clock, stats prometheus.Registerer, opts ...otelhttp.Option) http.Handler {
	responseTime := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "response_time",
			Help: "Time taken to respond to a request",
		},
		[]string{"endpoint", "method", "code"})
	stats.MustRegister(responseTime)
	return otelhttp.NewHandler(&MeasuredHandler{
		serveMux: m,
		clk:      clk,
		stat:     responseTime,
	}, "server", opts...)
}

func (h *MeasuredHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	begin := h.clk.Now()
	rwws := &responseWriterWithStatus{w, 0}

	// Use the method string only if it's a recognized HTTP method. This avoids
	// ballooning timeseries with invalid methods from public input.
	var method string
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut,
		http.MethodPatch, http.MethodDelete, http.MethodConnect,
		http.MethodOptions, http.MethodTrace:
		method = r.Method
	default:
		method = "unknown"
	}

	subHandler, pattern := h.Handler(r)
	defer func() {
		h.stat.With(prometheus.Labels{
			"endpoint": pattern,
			"method":   method,
			"code":     strconv.Itoa(rwws.code),
		}).Observe(h.clk.Since(begin).Seconds())
	}()

	subHandler.ServeHTTP(rwws, r)
}
