package probers

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/letsencrypt/boulder/observer/obsdialer"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/crypto/ocsp"
)

type reason int

const (
	none reason = iota
	internalError
	ocspError
	rootDidNotMatch
	responseDidNotMatch
)

var reasonToString = map[reason]string{
	none:                "nil",
	internalError:       "internalError",
	ocspError:           "ocspError",
	rootDidNotMatch:     "rootDidNotMatch",
	responseDidNotMatch: "responseDidNotMatch",
}

func getReasons() []string {
	var allReasons []string
	for _, v := range reasonToString {
		allReasons = append(allReasons, v)
	}
	return allReasons
}

// TLSProbe is the exported `Prober` object for monitors configured to perform
// TLS protocols.
type TLSProbe struct {
	hostname  string
	rootOrg   string
	rootCN    string
	response  string
	notAfter  *prometheus.GaugeVec
	notBefore *prometheus.GaugeVec
	reason    *prometheus.CounterVec
}

// Name returns a string that uniquely identifies the monitor.
func (p TLSProbe) Name() string {
	return p.hostname
}

// Kind returns a name that uniquely identifies the `Kind` of `Prober`.
func (p TLSProbe) Kind() string {
	return "TLS"
}

// Get OCSP status (good, revoked or unknown) of certificate
func checkOCSP(cert, issuer *x509.Certificate, want int) (bool, error) {
	req, err := ocsp.CreateRequest(cert, issuer, nil)
	if err != nil {
		return false, err
	}

	url := fmt.Sprintf("%s/%s", cert.OCSPServer[0], base64.StdEncoding.EncodeToString(req))
	res, err := http.Get(url)
	if err != nil {
		return false, err
	}

	output, err := io.ReadAll(res.Body)
	if err != nil {
		return false, err
	}

	ocspRes, err := ocsp.ParseResponseForCert(output, cert, issuer)
	if err != nil {
		return false, err
	}

	return ocspRes.Status == want, nil
}

// Return an error if the root settings are nonempty and do not match the
// expected root.
func (p TLSProbe) checkRoot(rootOrg, rootCN string) error {
	if (p.rootCN == "" && p.rootOrg == "") || (rootOrg == p.rootOrg && rootCN == p.rootCN) {
		return nil
	}
	return fmt.Errorf("Expected root does not match.")
}

// Export expiration timestamp and reason to Prometheus.
func (p TLSProbe) exportMetrics(cert *x509.Certificate, reason reason) {
	if cert != nil {
		p.notAfter.WithLabelValues(p.hostname).Set(float64(cert.NotAfter.Unix()))
		p.notBefore.WithLabelValues(p.hostname).Set(float64(cert.NotBefore.Unix()))
	}
	p.reason.WithLabelValues(p.hostname, reasonToString[reason]).Inc()
}

func (p TLSProbe) probeExpired(timeout time.Duration) bool {
	config := &tls.Config{
		// Set InsecureSkipVerify to skip the default validation we are
		// replacing. This will not disable VerifyConnection.
		InsecureSkipVerify: true,
		VerifyConnection: func(cs tls.ConnectionState) error {
			opts := x509.VerifyOptions{
				CurrentTime:   cs.PeerCertificates[0].NotAfter,
				Intermediates: x509.NewCertPool(),
			}
			for _, cert := range cs.PeerCertificates[1:] {
				opts.Intermediates.AddCert(cert)
			}
			_, err := cs.PeerCertificates[0].Verify(opts)
			return err
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	tlsDialer := tls.Dialer{
		NetDialer: &obsdialer.Dialer,
		Config:    config,
	}
	conn, err := tlsDialer.DialContext(ctx, "tcp", p.hostname+":443")
	if err != nil {
		p.exportMetrics(nil, internalError)
		return false
	}
	defer conn.Close()

	// tls.Dialer.DialContext is documented to always return *tls.Conn
	tlsConn := conn.(*tls.Conn)
	peers := tlsConn.ConnectionState().PeerCertificates
	if time.Until(peers[0].NotAfter) > 0 {
		p.exportMetrics(peers[0], responseDidNotMatch)
		return false
	}

	root := peers[len(peers)-1].Issuer
	err = p.checkRoot(root.Organization[0], root.CommonName)
	if err != nil {
		p.exportMetrics(peers[0], rootDidNotMatch)
		return false
	}

	p.exportMetrics(peers[0], none)
	return true
}

func (p TLSProbe) probeUnexpired(timeout time.Duration) bool {
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: timeout}, "tcp", p.hostname+":443", &tls.Config{})
	if err != nil {
		p.exportMetrics(nil, internalError)
		return false
	}

	defer conn.Close()
	peers := conn.ConnectionState().PeerCertificates
	root := peers[len(peers)-1].Issuer
	err = p.checkRoot(root.Organization[0], root.CommonName)
	if err != nil {
		p.exportMetrics(peers[0], rootDidNotMatch)
		return false
	}

	var ocspStatus bool
	switch p.response {
	case "valid":
		ocspStatus, err = checkOCSP(peers[0], peers[1], ocsp.Good)
	case "revoked":
		ocspStatus, err = checkOCSP(peers[0], peers[1], ocsp.Revoked)
	}
	if err != nil {
		p.exportMetrics(peers[0], ocspError)
		return false
	}

	if !ocspStatus {
		p.exportMetrics(peers[0], responseDidNotMatch)
		return false
	}

	p.exportMetrics(peers[0], none)
	return true
}

// Probe performs the configured TLS probe. Return true if the root has the
// expected Subject (or if no root is provided for comparison in settings), and
// the end entity certificate has the correct expiration status (either expired
// or unexpired, depending on what is configured). Exports metrics for the
// NotAfter timestamp of the end entity certificate and the reason for the Probe
// returning false ("none" if returns true).
func (p TLSProbe) Probe(timeout time.Duration) (bool, time.Duration) {
	start := time.Now()
	var success bool
	if p.response == "expired" {
		success = p.probeExpired(timeout)
	} else {
		success = p.probeUnexpired(timeout)
	}

	return success, time.Since(start)
}
