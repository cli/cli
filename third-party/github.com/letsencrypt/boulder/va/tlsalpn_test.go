package va

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/hex"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/letsencrypt/boulder/bdns"
	"github.com/letsencrypt/boulder/core"
	"github.com/letsencrypt/boulder/identifier"
	"github.com/letsencrypt/boulder/probs"
	"github.com/letsencrypt/boulder/test"
)

// acmeExtension returns the ACME TLS-ALPN-01 extension for the given key
// authorization. The OID can also be changed for the sake of testing.
func acmeExtension(oid asn1.ObjectIdentifier, keyAuthorization string) pkix.Extension {
	shasum := sha256.Sum256([]byte(keyAuthorization))
	encHash, _ := asn1.Marshal(shasum[:])
	return pkix.Extension{
		Id:       oid,
		Critical: true,
		Value:    encHash,
	}
}

// testACMEExt is the ACME TLS-ALPN-01 extension with the default OID and
// key authorization used in most tests.
var testACMEExt = acmeExtension(IdPeAcmeIdentifier, expectedKeyAuthorization)

// testTLSCert returns a ready-to-use self-signed certificate with the given
// SANs and Extensions. It generates a new ECDSA key on each call.
func testTLSCert(names []string, ips []net.IP, extensions []pkix.Extension) *tls.Certificate {
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1337),
		Subject: pkix.Name{
			Organization: []string{"tests"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().AddDate(0, 0, 1),

		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,

		DNSNames:        names,
		IPAddresses:     ips,
		ExtraExtensions: extensions,
	}
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	certBytes, _ := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)

	return &tls.Certificate{
		Certificate: [][]byte{certBytes},
		PrivateKey:  key,
	}
}

// testACMECert returns a certificate with the correctly-formed ACME TLS-ALPN-01
// extension with our default test values. Use acmeExtension and testCert if you
// need to customize the contents of that extension.
func testACMECert(names []string) *tls.Certificate {
	return testTLSCert(names, nil, []pkix.Extension{testACMEExt})
}

// tlsalpn01SrvWithCert creates a test server which will present the given
// certificate when asked to do a tls-alpn-01 handshake.
func tlsalpn01SrvWithCert(t *testing.T, acmeCert *tls.Certificate, tlsVersion uint16, ipv6 bool) *httptest.Server {
	t.Helper()

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{},
		ClientAuth:   tls.NoClientCert,
		GetCertificate: func(clientHello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			return acmeCert, nil
		},
		NextProtos: []string{"http/1.1", ACMETLS1Protocol},
		MinVersion: tlsVersion,
		MaxVersion: tlsVersion,
	}

	hs := httptest.NewUnstartedServer(http.DefaultServeMux)
	hs.TLS = tlsConfig
	hs.Config.TLSNextProto = map[string]func(*http.Server, *tls.Conn, http.Handler){
		ACMETLS1Protocol: func(_ *http.Server, conn *tls.Conn, _ http.Handler) {
			_ = conn.Close()
		},
	}
	if ipv6 {
		l, err := net.Listen("tcp", "[::1]:0")
		if err != nil {
			panic(fmt.Sprintf("httptest: failed to listen on a port: %v", err))
		}
		hs.Listener = l
	}
	hs.StartTLS()
	return hs
}

// testTLSALPN01Srv creates a test server with all default values, for tests
// that don't need to customize specific names or extensions in the certificate
// served by the TLS server.
func testTLSALPN01Srv(t *testing.T) *httptest.Server {
	return tlsalpn01SrvWithCert(t, testACMECert([]string{"expected"}), 0, false)
}

func slowTLSSrv() *httptest.Server {
	cert := testTLSCert([]string{"nomatter"}, nil, nil)
	server := httptest.NewUnstartedServer(http.DefaultServeMux)
	server.TLS = &tls.Config{
		NextProtos: []string{"http/1.1", ACMETLS1Protocol},
		GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
			time.Sleep(100 * time.Millisecond)
			return cert, nil
		},
	}
	server.StartTLS()
	return server
}

func TestTLSALPNTimeoutAfterConnect(t *testing.T) {
	hs := slowTLSSrv()
	va, _ := setup(hs, "", nil, nil)

	timeout := 50 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	started := time.Now()
	_, err := va.validateTLSALPN01(ctx, identifier.NewDNS("slow.server"), expectedKeyAuthorization)
	if err == nil {
		t.Fatalf("Validation should've failed")
	}
	// Check that the TLS connection doesn't return before a timeout, and times
	// out after the expected time
	took := time.Since(started)
	// Check that the HTTP connection doesn't return too fast, and times
	// out after the expected time
	if took < timeout/2 {
		t.Fatalf("TLSSNI returned before %s (%s) with %#v", timeout, took, err)
	}
	if took > 2*timeout {
		t.Fatalf("TLSSNI didn't timeout after %s (took %s to return %#v)", timeout,
			took, err)
	}
	if err == nil {
		t.Fatalf("Connection should've timed out")
	}
	prob := detailedError(err)
	test.AssertEquals(t, prob.Type, probs.ConnectionProblem)

	expected := "127.0.0.1: Timeout after connect (your server may be slow or overloaded)"
	if prob.Detail != expected {
		t.Errorf("Wrong error detail. Expected %q, got %q", expected, prob.Detail)
	}
}

func TestTLSALPN01DialTimeout(t *testing.T) {
	hs := slowTLSSrv()
	va, _ := setup(hs, "", nil, dnsMockReturnsUnroutable{&bdns.MockClient{}})
	started := time.Now()

	timeout := 50 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// The only method I've found so far to trigger a connect timeout is to
	// connect to an unrouteable IP address. This usually generates a connection
	// timeout, but will rarely return "Network unreachable" instead. If we get
	// that, just retry until we get something other than "Network unreachable".
	var err error
	for range 20 {
		_, err = va.validateTLSALPN01(ctx, identifier.NewDNS("unroutable.invalid"), expectedKeyAuthorization)
		if err != nil && strings.Contains(err.Error(), "Network unreachable") {
			continue
		} else {
			break
		}
	}

	if err == nil {
		t.Fatalf("Validation should've failed")
	}
	// Check that the TLS connection doesn't return before a timeout, and times
	// out after the expected time
	took := time.Since(started)
	// Check that the HTTP connection doesn't return too fast, and times
	// out after the expected time
	if took < timeout/2 {
		t.Fatalf("TLSSNI returned before %s (%s) with %#v", timeout, took, err)
	}
	if took > 2*timeout {
		t.Fatalf("TLSSNI didn't timeout after %s", timeout)
	}
	if err == nil {
		t.Fatalf("Connection should've timed out")
	}
	prob := detailedError(err)
	test.AssertEquals(t, prob.Type, probs.ConnectionProblem)
	expected := "64.112.117.254: Timeout during connect (likely firewall problem)"
	if prob.Detail != expected {
		t.Errorf("Wrong error detail. Expected %q, got %q", expected, prob.Detail)
	}
}

func TestTLSALPN01Refused(t *testing.T) {
	hs := testTLSALPN01Srv(t)

	va, _ := setup(hs, "", nil, nil)

	// Take down validation server and check that validation fails.
	hs.Close()
	_, err := va.validateTLSALPN01(ctx, identifier.NewDNS("expected"), expectedKeyAuthorization)
	if err == nil {
		t.Fatalf("Server's down; expected refusal. Where did we connect?")
	}
	prob := detailedError(err)
	test.AssertEquals(t, prob.Type, probs.ConnectionProblem)
	expected := "127.0.0.1: Connection refused"
	if prob.Detail != expected {
		t.Errorf("Wrong error detail. Expected %q, got %q", expected, prob.Detail)
	}
}

func TestTLSALPN01TalkingToHTTP(t *testing.T) {
	hs := testTLSALPN01Srv(t)

	va, _ := setup(hs, "", nil, nil)

	// Make the server only speak HTTP.
	httpOnly := httpSrv(t, "", false)
	va.tlsPort = getPort(httpOnly)

	_, err := va.validateTLSALPN01(ctx, identifier.NewDNS("expected"), expectedKeyAuthorization)
	test.AssertError(t, err, "TLS-SNI-01 validation passed when talking to a HTTP-only server")
	prob := detailedError(err)
	expected := "Server only speaks HTTP, not TLS"
	if !strings.HasSuffix(prob.String(), expected) {
		t.Errorf("Got wrong error detail. Expected %q, got %q", expected, prob)
	}
}

func brokenTLSSrv() *httptest.Server {
	server := httptest.NewUnstartedServer(http.DefaultServeMux)
	server.TLS = &tls.Config{
		GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
			return nil, fmt.Errorf("Failing on purpose")
		},
	}
	server.StartTLS()
	return server
}

func TestTLSError(t *testing.T) {
	hs := brokenTLSSrv()

	va, _ := setup(hs, "", nil, nil)

	_, err := va.validateTLSALPN01(ctx, identifier.NewDNS("expected"), expectedKeyAuthorization)
	if err == nil {
		t.Fatalf("TLS validation should have failed: What cert was used?")
	}
	prob := detailedError(err)
	if prob.Type != probs.TLSProblem {
		t.Errorf("Wrong problem type: got %s, expected type %s",
			prob, probs.TLSProblem)
	}
}

func TestDNSError(t *testing.T) {
	hs := brokenTLSSrv()

	va, _ := setup(hs, "", nil, nil)

	_, err := va.validateTLSALPN01(ctx, identifier.NewDNS("always.invalid"), expectedKeyAuthorization)
	if err == nil {
		t.Fatalf("TLS validation should have failed: what IP was used?")
	}
	prob := detailedError(err)
	if prob.Type != probs.DNSProblem {
		t.Errorf("Wrong problem type: got %s, expected type %s",
			prob, probs.DNSProblem)
	}
}

func TestCertNames(t *testing.T) {
	uri, err := url.Parse("ftp://something.else:1234")
	test.AssertNotError(t, err, "failed to parse fake URI")

	// We duplicate names inside the fields corresponding to the SAN set
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1337),
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(0, 0, 1),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,

		Subject: pkix.Name{
			// We also duplicate a name from the SANs as the CN
			CommonName: "hello.world",
		},
		DNSNames: []string{
			"hello.world", "goodbye.world",
			"hello.world", "goodbye.world",
			"bonjour.le.monde", "au.revoir.le.monde",
			"bonjour.le.monde", "au.revoir.le.monde",
		},
		EmailAddresses: []string{
			"hello@world.gov", "hello@world.gov",
		},
		IPAddresses: []net.IP{
			net.ParseIP("192.168.0.1"), net.ParseIP("192.168.0.1"),
			net.ParseIP("2001:db8::68"), net.ParseIP("2001:db8::68"),
		},
		URIs: []*url.URL{
			uri, uri,
		},
	}

	// Round-trip the certificate through generation and parsing, to make sure
	// certAltNames can handle "real" certificates and not just templates.
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	test.AssertNotError(t, err, "Error creating test key")
	certBytes, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	test.AssertNotError(t, err, "Error creating certificate")

	cert, err := x509.ParseCertificate(certBytes)
	test.AssertNotError(t, err, "Error parsing certificate")

	// We expect only unique names, in sorted order.
	expected := []string{
		"192.168.0.1",
		"2001:db8::68",
		"au.revoir.le.monde",
		"bonjour.le.monde",
		"ftp://something.else:1234",
		"goodbye.world",
		"hello.world",
		"hello@world.gov",
	}

	actual := certAltNames(cert)
	test.AssertDeepEquals(t, actual, expected)
}

func TestTLSALPN01SuccessDNS(t *testing.T) {
	hs := testTLSALPN01Srv(t)

	va, _ := setup(hs, "", nil, nil)

	_, err := va.validateTLSALPN01(ctx, identifier.NewDNS("expected"), expectedKeyAuthorization)
	if err != nil {
		t.Errorf("Validation failed: %v", err)
	}
	test.AssertMetricWithLabelsEquals(
		t, va.metrics.tlsALPNOIDCounter, prometheus.Labels{"oid": IdPeAcmeIdentifier.String()}, 1)

	hs.Close()
}

func TestTLSALPN01SuccessIPv4(t *testing.T) {
	cert := testTLSCert(nil, []net.IP{net.ParseIP("127.0.0.1")}, []pkix.Extension{testACMEExt})
	hs := tlsalpn01SrvWithCert(t, cert, 0, false)

	va, _ := setup(hs, "", nil, nil)

	_, err := va.validateTLSALPN01(ctx, identifier.NewIP(netip.MustParseAddr("127.0.0.1")), expectedKeyAuthorization)
	if err != nil {
		t.Errorf("Validation failed: %v", err)
	}
	test.AssertMetricWithLabelsEquals(
		t, va.metrics.tlsALPNOIDCounter, prometheus.Labels{"oid": IdPeAcmeIdentifier.String()}, 1)

	hs.Close()
}

func TestTLSALPN01SuccessIPv6(t *testing.T) {
	cert := testTLSCert(nil, []net.IP{net.ParseIP("::1")}, []pkix.Extension{testACMEExt})
	hs := tlsalpn01SrvWithCert(t, cert, 0, true)

	va, _ := setup(hs, "", nil, nil)

	_, err := va.validateTLSALPN01(ctx, identifier.NewIP(netip.MustParseAddr("::1")), expectedKeyAuthorization)
	if err != nil {
		t.Errorf("Validation failed: %v", err)
	}
	test.AssertMetricWithLabelsEquals(
		t, va.metrics.tlsALPNOIDCounter, prometheus.Labels{"oid": IdPeAcmeIdentifier.String()}, 1)

	hs.Close()
}

func TestTLSALPN01ObsoleteFailure(t *testing.T) {
	// NOTE: unfortunately another document claimed the OID we were using in
	// draft-ietf-acme-tls-alpn-01 for their own extension and IANA chose to
	// assign it early. Because of this we had to increment the
	// id-pe-acmeIdentifier OID. We supported this obsolete OID for a long time,
	// but no longer do so.
	// As defined in https://tools.ietf.org/html/draft-ietf-acme-tls-alpn-01#section-5.1
	// id-pe OID + 30 (acmeIdentifier) + 1 (v1)
	IdPeAcmeIdentifierV1Obsolete := asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 1, 30, 1}

	cert := testTLSCert([]string{"expected"}, nil, []pkix.Extension{acmeExtension(IdPeAcmeIdentifierV1Obsolete, expectedKeyAuthorization)})
	hs := tlsalpn01SrvWithCert(t, cert, 0, false)

	va, _ := setup(hs, "", nil, nil)

	_, err := va.validateTLSALPN01(ctx, identifier.NewDNS("expected"), expectedKeyAuthorization)
	test.AssertNotNil(t, err, "expected validation to fail")
	test.AssertContains(t, err.Error(), "Required extension OID 1.3.6.1.5.5.7.1.31 is not present")
}

func TestValidateTLSALPN01BadChallenge(t *testing.T) {
	badKeyAuthorization := ka("bad token")

	cert := testTLSCert([]string{"expected"}, nil, []pkix.Extension{acmeExtension(IdPeAcmeIdentifier, badKeyAuthorization)})
	hs := tlsalpn01SrvWithCert(t, cert, 0, false)

	va, _ := setup(hs, "", nil, nil)

	_, err := va.validateTLSALPN01(ctx, identifier.NewDNS("expected"), expectedKeyAuthorization)
	if err == nil {
		t.Fatalf("TLS ALPN validation should have failed.")
	}

	prob := detailedError(err)
	test.AssertEquals(t, prob.Type, probs.UnauthorizedProblem)

	expectedDigest := sha256.Sum256([]byte(expectedKeyAuthorization))
	badDigest := sha256.Sum256([]byte(badKeyAuthorization))

	test.AssertContains(t, err.Error(), string(core.ChallengeTypeTLSALPN01))
	test.AssertContains(t, err.Error(), hex.EncodeToString(expectedDigest[:]))
	test.AssertContains(t, err.Error(), hex.EncodeToString(badDigest[:]))
}

func TestValidateTLSALPN01BrokenSrv(t *testing.T) {
	hs := brokenTLSSrv()

	va, _ := setup(hs, "", nil, nil)

	_, err := va.validateTLSALPN01(ctx, identifier.NewDNS("expected"), expectedKeyAuthorization)
	if err == nil {
		t.Fatalf("TLS ALPN validation should have failed.")
	}
	prob := detailedError(err)
	test.AssertEquals(t, prob.Type, probs.TLSProblem)
}

func TestValidateTLSALPN01UnawareSrv(t *testing.T) {
	cert := testTLSCert([]string{"expected"}, nil, nil)
	hs := httptest.NewUnstartedServer(http.DefaultServeMux)
	hs.TLS = &tls.Config{
		Certificates: []tls.Certificate{},
		ClientAuth:   tls.NoClientCert,
		GetCertificate: func(clientHello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			return cert, nil
		},
		NextProtos: []string{"http/1.1"}, // Doesn't list ACMETLS1Protocol
	}
	hs.StartTLS()

	va, _ := setup(hs, "", nil, nil)

	_, err := va.validateTLSALPN01(ctx, identifier.NewDNS("expected"), expectedKeyAuthorization)
	if err == nil {
		t.Fatalf("TLS ALPN validation should have failed.")
	}
	prob := detailedError(err)
	test.AssertEquals(t, prob.Type, probs.TLSProblem)
}

// TestValidateTLSALPN01MalformedExtnValue tests that validating TLS-ALPN-01
// against a host that returns a certificate that contains an ASN.1 DER
// acmeValidation extension value that does not parse or is the wrong length
// will result in an Unauthorized problem
func TestValidateTLSALPN01MalformedExtnValue(t *testing.T) {
	wrongTypeDER, _ := asn1.Marshal("a string")
	wrongLengthDER, _ := asn1.Marshal(make([]byte, 31))
	badExtensions := []pkix.Extension{
		{
			Id:       IdPeAcmeIdentifier,
			Critical: true,
			Value:    wrongTypeDER,
		},
		{
			Id:       IdPeAcmeIdentifier,
			Critical: true,
			Value:    wrongLengthDER,
		},
	}

	for _, badExt := range badExtensions {
		acmeCert := testTLSCert([]string{"expected"}, nil, []pkix.Extension{badExt})
		hs := tlsalpn01SrvWithCert(t, acmeCert, 0, false)
		va, _ := setup(hs, "", nil, nil)

		_, err := va.validateTLSALPN01(ctx, identifier.NewDNS("expected"), expectedKeyAuthorization)
		hs.Close()

		if err == nil {
			t.Errorf("TLS ALPN validation should have failed for acmeValidation extension %+v.",
				badExt)
			continue
		}
		prob := detailedError(err)
		test.AssertEquals(t, prob.Type, probs.UnauthorizedProblem)
		test.AssertContains(t, prob.Detail, string(core.ChallengeTypeTLSALPN01))
		test.AssertContains(t, prob.Detail, "malformed acmeValidationV1 extension value")
	}
}

func TestTLSALPN01TLSVersion(t *testing.T) {
	cert := testACMECert([]string{"expected"})

	for _, tc := range []struct {
		version     uint16
		expectError bool
	}{
		{
			version:     tls.VersionTLS11,
			expectError: true,
		},
		{
			version:     tls.VersionTLS12,
			expectError: false,
		},
		{
			version:     tls.VersionTLS13,
			expectError: false,
		},
	} {
		// Create a server that only negotiates the given TLS version
		hs := tlsalpn01SrvWithCert(t, cert, tc.version, false)

		va, _ := setup(hs, "", nil, nil)

		_, err := va.validateTLSALPN01(ctx, identifier.NewDNS("expected"), expectedKeyAuthorization)
		if !tc.expectError {
			if err != nil {
				t.Errorf("expected success, got: %v", err)
			}
			// The correct TLS-ALPN-01 OID counter should have been incremented
			test.AssertMetricWithLabelsEquals(
				t, va.metrics.tlsALPNOIDCounter, prometheus.Labels{"oid": IdPeAcmeIdentifier.String()}, 1)
		} else {
			test.AssertNotNil(t, err, "expected validation error")
			test.AssertContains(t, err.Error(), "protocol version not supported")
			test.AssertMetricWithLabelsEquals(
				t, va.metrics.tlsALPNOIDCounter, prometheus.Labels{"oid": IdPeAcmeIdentifier.String()}, 0)
		}

		hs.Close()
	}
}

func TestTLSALPN01WrongName(t *testing.T) {
	// Create a cert with a different name from what we're validating
	hs := tlsalpn01SrvWithCert(t, testACMECert([]string{"incorrect"}), 0, false)

	va, _ := setup(hs, "", nil, nil)

	_, err := va.validateTLSALPN01(ctx, identifier.NewDNS("expected"), expectedKeyAuthorization)
	test.AssertError(t, err, "validation should have failed")
	test.AssertContains(t, err.Error(), "identifier does not match expected identifier")
}

func TestTLSALPN01WrongIPv4(t *testing.T) {
	// Create a cert with a different IP address from what we're validating
	cert := testTLSCert(nil, []net.IP{net.ParseIP("10.10.10.10")}, []pkix.Extension{testACMEExt})
	hs := tlsalpn01SrvWithCert(t, cert, 0, false)

	va, _ := setup(hs, "", nil, nil)

	_, err := va.validateTLSALPN01(ctx, identifier.NewIP(netip.MustParseAddr("127.0.0.1")), expectedKeyAuthorization)
	test.AssertError(t, err, "validation should have failed")
	test.AssertContains(t, err.Error(), "identifier does not match expected identifier")
}

func TestTLSALPN01WrongIPv6(t *testing.T) {
	// Create a cert with a different IP address from what we're validating
	cert := testTLSCert(nil, []net.IP{net.ParseIP("::2")}, []pkix.Extension{testACMEExt})
	hs := tlsalpn01SrvWithCert(t, cert, 0, true)

	va, _ := setup(hs, "", nil, nil)

	_, err := va.validateTLSALPN01(ctx, identifier.NewIP(netip.MustParseAddr("::1")), expectedKeyAuthorization)
	test.AssertError(t, err, "validation should have failed")
	test.AssertContains(t, err.Error(), "identifier does not match expected identifier")
}

func TestTLSALPN01ExtraNames(t *testing.T) {
	// Create a cert with two names when we only want to validate one.
	hs := tlsalpn01SrvWithCert(t, testACMECert([]string{"expected", "extra"}), 0, false)

	va, _ := setup(hs, "", nil, nil)

	_, err := va.validateTLSALPN01(ctx, identifier.NewDNS("expected"), expectedKeyAuthorization)
	test.AssertError(t, err, "validation should have failed")
	test.AssertContains(t, err.Error(), "wrong number of identifiers")
}

func TestTLSALPN01WrongIdentType(t *testing.T) {
	// Create a cert with an IP address encoded as a name.
	hs := tlsalpn01SrvWithCert(t, testACMECert([]string{"127.0.0.1"}), 0, false)

	va, _ := setup(hs, "", nil, nil)

	_, err := va.validateTLSALPN01(ctx, identifier.NewIP(netip.MustParseAddr("127.0.0.1")), expectedKeyAuthorization)
	test.AssertError(t, err, "validation should have failed")
	test.AssertContains(t, err.Error(), "wrong number of identifiers")
}

func TestTLSALPN01TooManyIdentTypes(t *testing.T) {
	// Create a cert with both a name and an IP address when we only want to validate one.
	hs := tlsalpn01SrvWithCert(t, testTLSCert([]string{"expected"}, []net.IP{net.ParseIP("127.0.0.1")}, []pkix.Extension{testACMEExt}), 0, false)

	va, _ := setup(hs, "", nil, nil)

	_, err := va.validateTLSALPN01(ctx, identifier.NewDNS("expected"), expectedKeyAuthorization)
	test.AssertError(t, err, "validation should have failed")
	test.AssertContains(t, err.Error(), "wrong number of identifiers")

	_, err = va.validateTLSALPN01(ctx, identifier.NewIP(netip.MustParseAddr("127.0.0.1")), expectedKeyAuthorization)
	test.AssertError(t, err, "validation should have failed")
	test.AssertContains(t, err.Error(), "wrong number of identifiers")
}

func TestTLSALPN01NotSelfSigned(t *testing.T) {
	// Create a normal-looking cert. We don't use testTLSCert because we need to
	// control the issuer.
	eeTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1337),
		Subject: pkix.Name{
			Organization: []string{"tests"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().AddDate(0, 0, 1),

		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},

		DNSNames:        []string{"expected"},
		IPAddresses:     []net.IP{net.ParseIP("192.168.0.1")},
		ExtraExtensions: []pkix.Extension{testACMEExt},
	}

	eeKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	test.AssertNotError(t, err, "creating test key")

	issuerCert := &x509.Certificate{
		SerialNumber: big.NewInt(1234),
		Subject: pkix.Name{
			Organization: []string{"testissuer"},
		},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	issuerKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	test.AssertNotError(t, err, "creating test key")

	// Test that a cert with mismatched subject and issuer fields is rejected,
	// even though its signature is produced with the right (self-signed) key.
	certBytes, err := x509.CreateCertificate(rand.Reader, eeTemplate, issuerCert, eeKey.Public(), eeKey)
	test.AssertNotError(t, err, "failed to create acme-tls/1 cert")

	acmeCert := &tls.Certificate{
		Certificate: [][]byte{certBytes},
		PrivateKey:  eeKey,
	}

	hs := tlsalpn01SrvWithCert(t, acmeCert, 0, false)

	va, _ := setup(hs, "", nil, nil)

	_, err = va.validateTLSALPN01(ctx, identifier.NewDNS("expected"), expectedKeyAuthorization)
	test.AssertError(t, err, "validation should have failed")
	test.AssertContains(t, err.Error(), "not self-signed")

	// Test that a cert whose signature was produced by some other key is rejected,
	// even though its subject and issuer fields claim that it is self-signed.
	certBytes, err = x509.CreateCertificate(rand.Reader, eeTemplate, eeTemplate, eeKey.Public(), issuerKey)
	test.AssertNotError(t, err, "failed to create acme-tls/1 cert")

	acmeCert = &tls.Certificate{
		Certificate: [][]byte{certBytes},
		PrivateKey:  eeKey,
	}

	hs = tlsalpn01SrvWithCert(t, acmeCert, 0, false)

	va, _ = setup(hs, "", nil, nil)

	_, err = va.validateTLSALPN01(ctx, identifier.NewDNS("expected"), expectedKeyAuthorization)
	test.AssertError(t, err, "validation should have failed")
	test.AssertContains(t, err.Error(), "not self-signed")
}

func TestTLSALPN01ExtraIdentifiers(t *testing.T) {
	// Create a cert with an extra non-dnsName identifier. We don't use testTLSCert
	// because we need to set the IPAddresses field.
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1337),
		Subject: pkix.Name{
			Organization: []string{"tests"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().AddDate(0, 0, 1),

		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,

		DNSNames:        []string{"expected"},
		IPAddresses:     []net.IP{net.ParseIP("192.168.0.1")},
		ExtraExtensions: []pkix.Extension{testACMEExt},
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	test.AssertNotError(t, err, "creating test key")
	certBytes, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	test.AssertNotError(t, err, "failed to create acme-tls/1 cert")

	acmeCert := &tls.Certificate{
		Certificate: [][]byte{certBytes},
		PrivateKey:  key,
	}

	hs := tlsalpn01SrvWithCert(t, acmeCert, tls.VersionTLS12, false)

	va, _ := setup(hs, "", nil, nil)

	_, err = va.validateTLSALPN01(ctx, identifier.NewDNS("expected"), expectedKeyAuthorization)
	test.AssertError(t, err, "validation should have failed")
	test.AssertContains(t, err.Error(), "Received certificate with unexpected identifiers")
}

func TestTLSALPN01ExtraSANs(t *testing.T) {
	// Create a cert with multiple SAN extensions
	sanValue, err := asn1.Marshal([]asn1.RawValue{
		{Tag: 2, Class: 2, Bytes: []byte(`expected`)},
	})
	test.AssertNotError(t, err, "failed to marshal test SAN")

	subjectAltName := pkix.Extension{
		Id:       asn1.ObjectIdentifier{2, 5, 29, 17},
		Critical: false,
		Value:    sanValue,
	}

	extensions := []pkix.Extension{testACMEExt, subjectAltName, subjectAltName}
	hs := tlsalpn01SrvWithCert(t, testTLSCert([]string{"expected"}, nil, extensions), 0, false)

	va, _ := setup(hs, "", nil, nil)

	_, err = va.validateTLSALPN01(ctx, identifier.NewDNS("expected"), expectedKeyAuthorization)
	test.AssertError(t, err, "validation should have failed")
	// In go >= 1.19, the TLS client library detects that the certificate has
	// a duplicate extension and terminates the connection itself.
	prob := detailedError(err)
	test.AssertContains(t, prob.String(), "Error getting validation data")
}

func TestTLSALPN01ExtraAcmeExtensions(t *testing.T) {
	// Create a cert with multiple SAN extensions
	extensions := []pkix.Extension{testACMEExt, testACMEExt}
	hs := tlsalpn01SrvWithCert(t, testTLSCert([]string{"expected"}, nil, extensions), 0, false)

	va, _ := setup(hs, "", nil, nil)

	_, err := va.validateTLSALPN01(ctx, identifier.NewDNS("expected"), expectedKeyAuthorization)
	test.AssertError(t, err, "validation should have failed")
	// In go >= 1.19, the TLS client library detects that the certificate has
	// a duplicate extension and terminates the connection itself.
	prob := detailedError(err)
	test.AssertContains(t, prob.String(), "Error getting validation data")
}

func TestAcceptableExtensions(t *testing.T) {
	requireAcmeAndSAN := []asn1.ObjectIdentifier{
		IdPeAcmeIdentifier,
		IdCeSubjectAltName,
	}

	sanValue, err := asn1.Marshal([]asn1.RawValue{
		{Tag: 2, Class: 2, Bytes: []byte(`expected`)},
	})
	test.AssertNotError(t, err, "failed to marshal test SAN")
	subjectAltName := pkix.Extension{
		Id:       asn1.ObjectIdentifier{2, 5, 29, 17},
		Critical: false,
		Value:    sanValue,
	}

	acmeExtension := pkix.Extension{
		Id:       IdPeAcmeIdentifier,
		Critical: true,
		Value:    []byte{},
	}

	weirdExt := pkix.Extension{
		Id:       asn1.ObjectIdentifier{99, 99, 99, 99},
		Critical: false,
		Value:    []byte(`because I'm tacky`),
	}

	doubleAcmeExts := []pkix.Extension{subjectAltName, acmeExtension, acmeExtension}
	err = checkAcceptableExtensions(doubleAcmeExts, requireAcmeAndSAN)
	test.AssertError(t, err, "Two ACME extensions isn't okay")

	doubleSANExts := []pkix.Extension{subjectAltName, subjectAltName, acmeExtension}
	err = checkAcceptableExtensions(doubleSANExts, requireAcmeAndSAN)
	test.AssertError(t, err, "Two SAN extensions isn't okay")

	onlyUnexpectedExt := []pkix.Extension{weirdExt}
	err = checkAcceptableExtensions(onlyUnexpectedExt, requireAcmeAndSAN)
	test.AssertError(t, err, "Missing required extensions")
	test.AssertContains(t, err.Error(), "Required extension OID 1.3.6.1.5.5.7.1.31 is not present")

	okayExts := []pkix.Extension{acmeExtension, subjectAltName}
	err = checkAcceptableExtensions(okayExts, requireAcmeAndSAN)
	test.AssertNotError(t, err, "Correct type and number of extensions")

	okayWithUnexpectedExt := []pkix.Extension{weirdExt, acmeExtension, subjectAltName}
	err = checkAcceptableExtensions(okayWithUnexpectedExt, requireAcmeAndSAN)
	test.AssertNotError(t, err, "Correct type and number of extensions")
}

func TestTLSALPN01BadIdentifier(t *testing.T) {
	hs := httpSrv(t, expectedToken, false)
	defer hs.Close()

	va, _ := setup(hs, "", nil, nil)

	_, err := va.validateTLSALPN01(ctx, identifier.ACMEIdentifier{Type: "smime", Value: "dobber@bad.horse"}, expectedKeyAuthorization)
	test.AssertError(t, err, "Server accepted a hypothetical S/MIME identifier")
	prob := detailedError(err)
	test.AssertContains(t, prob.String(), "Identifier type for TLS-ALPN-01 challenge was not DNS or IP")
}

// TestTLSALPN01ServerName tests compliance with RFC 8737, Sec. 3 (step 3) & RFC
// 8738, Sec. 6.
func TestTLSALPN01ServerName(t *testing.T) {
	testCases := []struct {
		Name      string
		Ident     identifier.ACMEIdentifier
		CertNames []string
		CertIPs   []net.IP
		IPv6      bool
		want      string
	}{
		{
			Name:      "DNS name",
			Ident:     identifier.NewDNS("example.com"),
			CertNames: []string{"example.com"},
			want:      "example.com",
		},
		{
			// RFC 8738, Sec. 6.
			Name:    "IPv4 address",
			Ident:   identifier.NewIP(netip.MustParseAddr("127.0.0.1")),
			CertIPs: []net.IP{net.ParseIP("127.0.0.1")},
			want:    "1.0.0.127.in-addr.arpa",
		},
		{
			// RFC 8738, Sec. 6.
			Name:    "IPv6 address",
			Ident:   identifier.NewIP(netip.MustParseAddr("::1")),
			CertIPs: []net.IP{net.ParseIP("::1")},
			IPv6:    true,
			want:    "1.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.ip6.arpa",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*500)
			defer cancel()

			tlsConfig := &tls.Config{
				Certificates: []tls.Certificate{},
				ClientAuth:   tls.NoClientCert,
				NextProtos:   []string{"http/1.1", ACMETLS1Protocol},
				GetCertificate: func(clientHello *tls.ClientHelloInfo) (*tls.Certificate, error) {
					got := clientHello.ServerName
					if got != tc.want {
						return nil, fmt.Errorf("Got host %#v, but want %#v", got, tc.want)
					}
					return testTLSCert(tc.CertNames, tc.CertIPs, []pkix.Extension{testACMEExt}), nil
				},
			}

			hs := httptest.NewUnstartedServer(http.DefaultServeMux)
			hs.TLS = tlsConfig
			hs.Config.TLSNextProto = map[string]func(*http.Server, *tls.Conn, http.Handler){
				ACMETLS1Protocol: func(_ *http.Server, conn *tls.Conn, _ http.Handler) {
					_ = conn.Close()
				},
			}
			if tc.IPv6 {
				l, err := net.Listen("tcp", "[::1]:0")
				if err != nil {
					panic(fmt.Sprintf("httptest: failed to listen on a port: %v", err))
				}
				hs.Listener = l
			}
			hs.StartTLS()
			defer hs.Close()

			va, _ := setup(hs, "", nil, nil)

			// The actual test happens in the tlsConfig.GetCertificate function,
			// which the validation will call and depend on for its success.
			_, err := va.validateTLSALPN01(ctx, tc.Ident, expectedKeyAuthorization)
			if err != nil {
				t.Errorf("Validation failed: %v", err)
			}
		})
	}
}
