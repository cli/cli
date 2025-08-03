package probers

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/crypto/ocsp"

	"github.com/letsencrypt/boulder/observer/obsdialer"
)

type reason int

const (
	none reason = iota
	internalError
	revocationStatusError
	rootDidNotMatch
	statusDidNotMatch
)

var reasonToString = map[reason]string{
	none:                  "nil",
	internalError:         "internalError",
	revocationStatusError: "revocationStatusError",
	rootDidNotMatch:       "rootDidNotMatch",
	statusDidNotMatch:     "statusDidNotMatch",
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
func checkOCSP(ctx context.Context, cert, issuer *x509.Certificate, want int) (bool, error) {
	req, err := ocsp.CreateRequest(cert, issuer, nil)
	if err != nil {
		return false, err
	}

	url := fmt.Sprintf("%s/%s", cert.OCSPServer[0], base64.StdEncoding.EncodeToString(req))
	r, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false, err
	}

	res, err := http.DefaultClient.Do(r)
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

func checkCRL(ctx context.Context, cert, issuer *x509.Certificate, want int) (bool, error) {
	if len(cert.CRLDistributionPoints) != 1 {
		return false, errors.New("cert does not contain CRLDP URI")
	}

	req, err := http.NewRequestWithContext(ctx, "GET", cert.CRLDistributionPoints[0], nil)
	if err != nil {
		return false, fmt.Errorf("creating HTTP request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("downloading CRL: %w", err)
	}
	defer resp.Body.Close()

	der, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("reading CRL: %w", err)
	}

	crl, err := x509.ParseRevocationList(der)
	if err != nil {
		return false, fmt.Errorf("parsing CRL: %w", err)
	}

	err = crl.CheckSignatureFrom(issuer)
	if err != nil {
		return false, fmt.Errorf("validating CRL: %w", err)
	}

	for _, entry := range crl.RevokedCertificateEntries {
		if entry.SerialNumber.Cmp(cert.SerialNumber) == 0 {
			return want == ocsp.Revoked, nil
		}
	}
	return want == ocsp.Good, nil
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
	addr := p.hostname
	_, _, err := net.SplitHostPort(addr)
	if err != nil {
		addr = net.JoinHostPort(addr, "443")
	}

	tlsDialer := tls.Dialer{
		NetDialer: &obsdialer.Dialer,
		Config: &tls.Config{
			// Set InsecureSkipVerify to skip the default validation we are
			// replacing. This will not disable VerifyConnection.
			InsecureSkipVerify: true,
			VerifyConnection: func(cs tls.ConnectionState) error {
				issuers := x509.NewCertPool()
				for _, cert := range cs.PeerCertificates[1:] {
					issuers.AddCert(cert)
				}
				opts := x509.VerifyOptions{
					// We set the current time to be the cert's expiration date so that
					// the validation routine doesn't complain that the cert is expired.
					CurrentTime: cs.PeerCertificates[0].NotAfter,
					// By settings roots and intermediates to be whatever was presented
					// in the handshake, we're saying that we don't care about the cert
					// chaining up to the system trust store. This is safe because we
					// check the root ourselves in checkRoot().
					Intermediates: issuers,
					Roots:         issuers,
				}
				_, err := cs.PeerCertificates[0].Verify(opts)
				return err
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	conn, err := tlsDialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		p.exportMetrics(nil, internalError)
		return false
	}
	defer conn.Close()

	// tls.Dialer.DialContext is documented to always return *tls.Conn
	peers := conn.(*tls.Conn).ConnectionState().PeerCertificates
	if time.Until(peers[0].NotAfter) > 0 {
		p.exportMetrics(peers[0], statusDidNotMatch)
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
	addr := p.hostname
	_, _, err := net.SplitHostPort(addr)
	if err != nil {
		addr = net.JoinHostPort(addr, "443")
	}

	tlsDialer := tls.Dialer{
		NetDialer: &obsdialer.Dialer,
		Config: &tls.Config{
			// Set InsecureSkipVerify to skip the default validation we are
			// replacing. This will not disable VerifyConnection.
			InsecureSkipVerify: true,
			VerifyConnection: func(cs tls.ConnectionState) error {
				issuers := x509.NewCertPool()
				for _, cert := range cs.PeerCertificates[1:] {
					issuers.AddCert(cert)
				}
				opts := x509.VerifyOptions{
					// By settings roots and intermediates to be whatever was presented
					// in the handshake, we're saying that we don't care about the cert
					// chaining up to the system trust store. This is safe because we
					// check the root ourselves in checkRoot().
					Intermediates: issuers,
					Roots:         issuers,
				}
				_, err := cs.PeerCertificates[0].Verify(opts)
				return err
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	conn, err := tlsDialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		p.exportMetrics(nil, internalError)
		return false
	}
	defer conn.Close()

	// tls.Dialer.DialContext is documented to always return *tls.Conn
	peers := conn.(*tls.Conn).ConnectionState().PeerCertificates
	root := peers[len(peers)-1].Issuer
	err = p.checkRoot(root.Organization[0], root.CommonName)
	if err != nil {
		p.exportMetrics(peers[0], rootDidNotMatch)
		return false
	}

	var wantStatus int
	switch p.response {
	case "valid":
		wantStatus = ocsp.Good
	case "revoked":
		wantStatus = ocsp.Revoked
	}

	var statusMatch bool
	if len(peers[0].OCSPServer) != 0 {
		statusMatch, err = checkOCSP(ctx, peers[0], peers[1], wantStatus)
	} else {
		statusMatch, err = checkCRL(ctx, peers[0], peers[1], wantStatus)
	}
	if err != nil {
		p.exportMetrics(peers[0], revocationStatusError)
		return false
	}

	if !statusMatch {
		p.exportMetrics(peers[0], statusDidNotMatch)
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
