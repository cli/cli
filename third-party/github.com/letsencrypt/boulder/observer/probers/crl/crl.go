package probers

import (
	"crypto/x509"
	"io"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// CRLProbe is the exported 'Prober' object for monitors configured to
// monitor CRL availability & characteristics.
type CRLProbe struct {
	url         string
	cNextUpdate *prometheus.GaugeVec
	cThisUpdate *prometheus.GaugeVec
	cCertCount  *prometheus.GaugeVec
}

// Name returns a string that uniquely identifies the monitor.
func (p CRLProbe) Name() string {
	return p.url
}

// Kind returns a name that uniquely identifies the `Kind` of `Prober`.
func (p CRLProbe) Kind() string {
	return "CRL"
}

// Probe requests the configured CRL and publishes metrics about it if found.
func (p CRLProbe) Probe(timeout time.Duration) (bool, time.Duration) {
	start := time.Now()
	resp, err := http.Get(p.url)
	if err != nil {
		return false, time.Since(start)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, time.Since(start)
	}
	dur := time.Since(start)

	crl, err := x509.ParseRevocationList(body)
	if err != nil {
		return false, dur
	}

	// Report metrics for this CRL
	p.cThisUpdate.WithLabelValues(p.url).Set(float64(crl.ThisUpdate.Unix()))
	p.cNextUpdate.WithLabelValues(p.url).Set(float64(crl.NextUpdate.Unix()))
	p.cCertCount.WithLabelValues(p.url).Set(float64(len(crl.RevokedCertificateEntries)))

	return true, dur
}
