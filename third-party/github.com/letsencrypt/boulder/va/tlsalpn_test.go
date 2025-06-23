package va

import (
	"context"
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
	"net/url"
	"strconv"
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

func tlsCertTemplate(names []string) *x509.Certificate {
	return &x509.Certificate{
		SerialNumber: big.NewInt(1337),
		Subject: pkix.Name{
			Organization: []string{"tests"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().AddDate(0, 0, 1),

		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,

		DNSNames: names,
	}
}

func makeACert(names []string) *tls.Certificate {
	template := tlsCertTemplate(names)
	certBytes, _ := x509.CreateCertificate(rand.Reader, template, template, &TheKey.PublicKey, &TheKey)
	return &tls.Certificate{
		Certificate: [][]byte{certBytes},
		PrivateKey:  &TheKey,
	}
}

// tlssniSrvWithNames is kept around for the use of TestValidateTLSALPN01UnawareSrv
func tlssniSrvWithNames(t *testing.T, names ...string) *httptest.Server {
	t.Helper()

	cert := makeACert(names)
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{*cert},
		ClientAuth:   tls.NoClientCert,
		GetCertificate: func(clientHello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			return cert, nil
		},
		NextProtos: []string{"http/1.1"},
	}

	hs := httptest.NewUnstartedServer(http.DefaultServeMux)
	hs.TLS = tlsConfig
	hs.StartTLS()
	return hs
}

func tlsalpn01SrvWithCert(t *testing.T, acmeCert *tls.Certificate, tlsVersion uint16) *httptest.Server {
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
	hs.StartTLS()
	return hs
}

func tlsalpn01Srv(
	t *testing.T,
	keyAuthorization string,
	oid asn1.ObjectIdentifier,
	tlsVersion uint16,
	names ...string) (*httptest.Server, error) {
	template := tlsCertTemplate(names)

	shasum := sha256.Sum256([]byte(keyAuthorization))
	encHash, err := asn1.Marshal(shasum[:])
	if err != nil {
		return nil, err
	}
	acmeExtension := pkix.Extension{
		Id:       oid,
		Critical: true,
		Value:    encHash,
	}
	template.ExtraExtensions = []pkix.Extension{acmeExtension}

	certBytes, err := x509.CreateCertificate(rand.Reader, template, template, &TheKey.PublicKey, &TheKey)
	if err != nil {
		return nil, err
	}

	acmeCert := &tls.Certificate{
		Certificate: [][]byte{certBytes},
		PrivateKey:  &TheKey,
	}

	return tlsalpn01SrvWithCert(t, acmeCert, tlsVersion), nil
}

func TestTLSALPN01FailIP(t *testing.T) {
	hs, err := tlsalpn01Srv(t, expectedKeyAuthorization, IdPeAcmeIdentifier, 0, "expected")
	test.AssertNotError(t, err, "Error creating test server")

	va, _ := setup(hs, 0, "", nil, nil)

	port := getPort(hs)
	_, err = va.validateTLSALPN01(ctx, identifier.ACMEIdentifier{
		Type:  identifier.IdentifierType("ip"),
		Value: net.JoinHostPort("127.0.0.1", strconv.Itoa(port)),
	}, expectedKeyAuthorization)
	if err == nil {
		t.Fatalf("IdentifierType IP shouldn't have worked.")
	}
	prob := detailedError(err)
	test.AssertEquals(t, prob.Type, probs.MalformedProblem)
}

func slowTLSSrv() *httptest.Server {
	server := httptest.NewUnstartedServer(http.DefaultServeMux)
	server.TLS = &tls.Config{
		NextProtos: []string{"http/1.1", ACMETLS1Protocol},
		GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
			time.Sleep(100 * time.Millisecond)
			return makeACert([]string{"nomatter"}), nil
		},
	}
	server.StartTLS()
	return server
}

func TestTLSALPNTimeoutAfterConnect(t *testing.T) {
	hs := slowTLSSrv()
	va, _ := setup(hs, 0, "", nil, nil)

	timeout := 50 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	started := time.Now()
	_, err := va.validateTLSALPN01(ctx, dnsi("slow.server"), expectedKeyAuthorization)
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
	va, _ := setup(hs, 0, "", nil, dnsMockReturnsUnroutable{&bdns.MockClient{}})
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
		_, err = va.validateTLSALPN01(ctx, dnsi("unroutable.invalid"), expectedKeyAuthorization)
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
	expected := "198.51.100.1: Timeout during connect (likely firewall problem)"
	if prob.Detail != expected {
		t.Errorf("Wrong error detail. Expected %q, got %q", expected, prob.Detail)
	}
}

func TestTLSALPN01Refused(t *testing.T) {
	hs, err := tlsalpn01Srv(t, expectedKeyAuthorization, IdPeAcmeIdentifier, 0, "expected")
	test.AssertNotError(t, err, "Error creating test server")

	va, _ := setup(hs, 0, "", nil, nil)
	// Take down validation server and check that validation fails.
	hs.Close()
	_, err = va.validateTLSALPN01(ctx, dnsi("expected"), expectedKeyAuthorization)
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
	hs, err := tlsalpn01Srv(t, expectedKeyAuthorization, IdPeAcmeIdentifier, 0, "expected")
	test.AssertNotError(t, err, "Error creating test server")

	va, _ := setup(hs, 0, "", nil, nil)
	httpOnly := httpSrv(t, "")
	va.tlsPort = getPort(httpOnly)

	_, err = va.validateTLSALPN01(ctx, dnsi("expected"), expectedKeyAuthorization)
	test.AssertError(t, err, "TLS-SNI-01 validation passed when talking to a HTTP-only server")
	prob := detailedError(err)
	expected := "Server only speaks HTTP, not TLS"
	if !strings.HasSuffix(prob.Error(), expected) {
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

	va, _ := setup(hs, 0, "", nil, nil)

	_, err := va.validateTLSALPN01(ctx, dnsi("expected"), expectedKeyAuthorization)
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

	va, _ := setup(hs, 0, "", nil, nil)

	_, err := va.validateTLSALPN01(ctx, dnsi("always.invalid"), expectedKeyAuthorization)
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

	// Create the certificate, check that certNames provides the expected result
	certBytes, err := x509.CreateCertificate(rand.Reader, template, template, &TheKey.PublicKey, &TheKey)
	test.AssertNotError(t, err, "Error creating certificate")

	cert, err := x509.ParseCertificate(certBytes)
	test.AssertNotError(t, err, "Error parsing certificate")

	actual := certAltNames(cert)
	test.AssertDeepEquals(t, actual, expected)
}

func TestTLSALPN01Success(t *testing.T) {
	hs, err := tlsalpn01Srv(t, expectedKeyAuthorization, IdPeAcmeIdentifier, 0, "expected")
	test.AssertNotError(t, err, "Error creating test server")

	va, _ := setup(hs, 0, "", nil, nil)

	_, prob := va.validateTLSALPN01(ctx, dnsi("expected"), expectedKeyAuthorization)
	if prob != nil {
		t.Errorf("Validation failed: %v", prob)
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

	hs, err := tlsalpn01Srv(t, expectedKeyAuthorization, IdPeAcmeIdentifierV1Obsolete, 0, "expected")
	test.AssertNotError(t, err, "Error creating test server")

	va, _ := setup(hs, 0, "", nil, nil)

	_, prob := va.validateTLSALPN01(ctx, dnsi("expected"), expectedKeyAuthorization)
	test.AssertNotNil(t, prob, "expected validation to fail")
}

func TestValidateTLSALPN01BadChallenge(t *testing.T) {
	badKeyAuthorization := ka("bad token")

	hs, err := tlsalpn01Srv(t, badKeyAuthorization, IdPeAcmeIdentifier, 0, "expected")
	test.AssertNotError(t, err, "Error creating test server")

	va, _ := setup(hs, 0, "", nil, nil)

	_, err = va.validateTLSALPN01(ctx, dnsi("expected"), expectedKeyAuthorization)

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

	va, _ := setup(hs, 0, "", nil, nil)

	_, err := va.validateTLSALPN01(ctx, dnsi("expected"), expectedKeyAuthorization)
	if err == nil {
		t.Fatalf("TLS ALPN validation should have failed.")
	}
	prob := detailedError(err)
	test.AssertEquals(t, prob.Type, probs.TLSProblem)
}

func TestValidateTLSALPN01UnawareSrv(t *testing.T) {
	hs := tlssniSrvWithNames(t, "expected")

	va, _ := setup(hs, 0, "", nil, nil)

	_, err := va.validateTLSALPN01(ctx, dnsi("expected"), expectedKeyAuthorization)
	if err == nil {
		t.Fatalf("TLS ALPN validation should have failed.")
	}
	prob := detailedError(err)
	test.AssertEquals(t, prob.Type, probs.TLSProblem)
}

// TestValidateTLSALPN01BadUTFSrv tests that validating TLS-ALPN-01 against
// a host that returns a certificate with a SAN/CN that contains invalid UTF-8
// will result in a problem with the invalid UTF-8.
func TestValidateTLSALPN01BadUTFSrv(t *testing.T) {
	_, err := tlsalpn01Srv(t, expectedKeyAuthorization, IdPeAcmeIdentifier, 0, "expected", "\xf0\x28\x8c\xbc")
	test.AssertContains(t, err.Error(), "cannot be encoded as an IA5String")
}

// TestValidateTLSALPN01MalformedExtnValue tests that validating TLS-ALPN-01
// against a host that returns a certificate that contains an ASN.1 DER
// acmeValidation extension value that does not parse or is the wrong length
// will result in an Unauthorized problem
func TestValidateTLSALPN01MalformedExtnValue(t *testing.T) {
	names := []string{"expected"}
	template := tlsCertTemplate(names)

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
		template.ExtraExtensions = []pkix.Extension{badExt}
		certBytes, _ := x509.CreateCertificate(rand.Reader, template, template, &TheKey.PublicKey, &TheKey)
		acmeCert := &tls.Certificate{
			Certificate: [][]byte{certBytes},
			PrivateKey:  &TheKey,
		}

		hs := tlsalpn01SrvWithCert(t, acmeCert, 0)
		va, _ := setup(hs, 0, "", nil, nil)

		_, err := va.validateTLSALPN01(ctx, dnsi("expected"), expectedKeyAuthorization)
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
		hs, err := tlsalpn01Srv(t, expectedKeyAuthorization, IdPeAcmeIdentifier, tc.version, "expected")
		test.AssertNotError(t, err, "Error creating test server")

		va, _ := setup(hs, 0, "", nil, nil)

		_, prob := va.validateTLSALPN01(ctx, dnsi("expected"), expectedKeyAuthorization)
		if !tc.expectError {
			if prob != nil {
				t.Errorf("expected success, got: %v", prob)
			}
			// The correct TLS-ALPN-01 OID counter should have been incremented
			test.AssertMetricWithLabelsEquals(
				t, va.metrics.tlsALPNOIDCounter, prometheus.Labels{"oid": IdPeAcmeIdentifier.String()}, 1)
		} else {
			test.AssertNotNil(t, prob, "expected validation error")
			test.AssertMetricWithLabelsEquals(
				t, va.metrics.tlsALPNOIDCounter, prometheus.Labels{"oid": IdPeAcmeIdentifier.String()}, 0)
		}

		hs.Close()
	}
}

func TestTLSALPN01WrongName(t *testing.T) {
	// Create a cert with a different name from what we're validating
	hs, err := tlsalpn01Srv(t, expectedKeyAuthorization, IdPeAcmeIdentifier, tls.VersionTLS12, "incorrect")
	test.AssertNotError(t, err, "failed to set up tls-alpn-01 server")

	va, _ := setup(hs, 0, "", nil, nil)

	_, prob := va.validateTLSALPN01(ctx, dnsi("expected"), expectedKeyAuthorization)
	test.AssertError(t, prob, "validation should have failed")
}

func TestTLSALPN01ExtraNames(t *testing.T) {
	// Create a cert with two names when we only want to validate one.
	hs, err := tlsalpn01Srv(t, expectedKeyAuthorization, IdPeAcmeIdentifier, tls.VersionTLS12, "expected", "extra")
	test.AssertNotError(t, err, "failed to set up tls-alpn-01 server")

	va, _ := setup(hs, 0, "", nil, nil)

	_, prob := va.validateTLSALPN01(ctx, dnsi("expected"), expectedKeyAuthorization)
	test.AssertError(t, prob, "validation should have failed")
}

func TestTLSALPN01NotSelfSigned(t *testing.T) {
	// Create a cert with an extra non-dnsName identifier.
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1337),
		Subject: pkix.Name{
			Organization: []string{"tests"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().AddDate(0, 0, 1),

		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},

		DNSNames:    []string{"expected"},
		IPAddresses: []net.IP{net.ParseIP("192.168.0.1")},
	}

	shasum := sha256.Sum256([]byte(expectedKeyAuthorization))
	encHash, err := asn1.Marshal(shasum[:])
	test.AssertNotError(t, err, "failed to create key authorization")

	acmeExtension := pkix.Extension{
		Id:       IdPeAcmeIdentifier,
		Critical: true,
		Value:    encHash,
	}
	template.ExtraExtensions = []pkix.Extension{acmeExtension}

	parent := &x509.Certificate{
		SerialNumber: big.NewInt(1234),
		Subject: pkix.Name{
			Organization: []string{"testissuer"},
		},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	// Note that this currently only tests that the subject and issuer are the
	// same; it does not test the case where the cert is signed by a different key.
	certBytes, err := x509.CreateCertificate(rand.Reader, template, parent, &TheKey.PublicKey, &TheKey)
	test.AssertNotError(t, err, "failed to create acme-tls/1 cert")

	acmeCert := &tls.Certificate{
		Certificate: [][]byte{certBytes},
		PrivateKey:  &TheKey,
	}

	hs := tlsalpn01SrvWithCert(t, acmeCert, tls.VersionTLS12)

	va, _ := setup(hs, 0, "", nil, nil)

	_, err = va.validateTLSALPN01(ctx, dnsi("expected"), expectedKeyAuthorization)
	test.AssertError(t, err, "validation should have failed")
	test.AssertContains(t, err.Error(), "not self-signed")
}

func TestTLSALPN01ExtraIdentifiers(t *testing.T) {
	// Create a cert with an extra non-dnsName identifier.
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1337),
		Subject: pkix.Name{
			Organization: []string{"tests"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().AddDate(0, 0, 1),

		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,

		DNSNames:    []string{"expected"},
		IPAddresses: []net.IP{net.ParseIP("192.168.0.1")},
	}

	shasum := sha256.Sum256([]byte(expectedKeyAuthorization))
	encHash, err := asn1.Marshal(shasum[:])
	test.AssertNotError(t, err, "failed to create key authorization")

	acmeExtension := pkix.Extension{
		Id:       IdPeAcmeIdentifier,
		Critical: true,
		Value:    encHash,
	}
	template.ExtraExtensions = []pkix.Extension{acmeExtension}
	certBytes, err := x509.CreateCertificate(rand.Reader, template, template, &TheKey.PublicKey, &TheKey)
	test.AssertNotError(t, err, "failed to create acme-tls/1 cert")

	acmeCert := &tls.Certificate{
		Certificate: [][]byte{certBytes},
		PrivateKey:  &TheKey,
	}

	hs := tlsalpn01SrvWithCert(t, acmeCert, tls.VersionTLS12)

	va, _ := setup(hs, 0, "", nil, nil)

	_, prob := va.validateTLSALPN01(ctx, dnsi("expected"), expectedKeyAuthorization)
	test.AssertError(t, prob, "validation should have failed")
}

func TestTLSALPN01ExtraSANs(t *testing.T) {
	// Create a cert with multiple SAN extensions
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1337),
		Subject: pkix.Name{
			Organization: []string{"tests"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().AddDate(0, 0, 1),

		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	shasum := sha256.Sum256([]byte(expectedKeyAuthorization))
	encHash, err := asn1.Marshal(shasum[:])
	test.AssertNotError(t, err, "failed to create key authorization")

	acmeExtension := pkix.Extension{
		Id:       IdPeAcmeIdentifier,
		Critical: true,
		Value:    encHash,
	}

	subjectAltName := pkix.Extension{}
	subjectAltName.Id = asn1.ObjectIdentifier{2, 5, 29, 17}
	subjectAltName.Critical = false
	subjectAltName.Value, err = asn1.Marshal([]asn1.RawValue{
		{Tag: 2, Class: 2, Bytes: []byte(`expected`)},
	})
	test.AssertNotError(t, err, "failed to marshal first SAN")

	extraSubjectAltName := pkix.Extension{}
	extraSubjectAltName.Id = asn1.ObjectIdentifier{2, 5, 29, 17}
	extraSubjectAltName.Critical = false
	extraSubjectAltName.Value, err = asn1.Marshal([]asn1.RawValue{
		{Tag: 2, Class: 2, Bytes: []byte(`expected`)},
	})
	test.AssertNotError(t, err, "failed to marshal extra SAN")

	template.ExtraExtensions = []pkix.Extension{acmeExtension, subjectAltName, extraSubjectAltName}
	certBytes, err := x509.CreateCertificate(rand.Reader, template, template, &TheKey.PublicKey, &TheKey)
	test.AssertNotError(t, err, "failed to create acme-tls/1 cert")

	acmeCert := &tls.Certificate{
		Certificate: [][]byte{certBytes},
		PrivateKey:  &TheKey,
	}

	hs := tlsalpn01SrvWithCert(t, acmeCert, tls.VersionTLS12)

	va, _ := setup(hs, 0, "", nil, nil)

	_, err = va.validateTLSALPN01(ctx, dnsi("expected"), expectedKeyAuthorization)
	test.AssertError(t, err, "validation should have failed")
	// In go >= 1.19, the TLS client library detects that the certificate has
	// a duplicate extension and terminates the connection itself.
	prob := detailedError(err)
	test.AssertContains(t, prob.Error(), "Error getting validation data")
}

func TestTLSALPN01ExtraAcmeExtensions(t *testing.T) {
	// Create a cert with multiple SAN extensions
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1337),
		Subject: pkix.Name{
			Organization: []string{"tests"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().AddDate(0, 0, 1),

		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,

		DNSNames: []string{"expected"},
	}

	shasum := sha256.Sum256([]byte(expectedKeyAuthorization))
	encHash, err := asn1.Marshal(shasum[:])
	test.AssertNotError(t, err, "failed to create key authorization")

	acmeExtension := pkix.Extension{
		Id:       IdPeAcmeIdentifier,
		Critical: true,
		Value:    encHash,
	}

	extraAcmeExtension := pkix.Extension{
		Id:       IdPeAcmeIdentifier,
		Critical: true,
		Value:    encHash,
	}

	template.ExtraExtensions = []pkix.Extension{acmeExtension, extraAcmeExtension}
	certBytes, err := x509.CreateCertificate(rand.Reader, template, template, &TheKey.PublicKey, &TheKey)
	test.AssertNotError(t, err, "failed to create acme-tls/1 cert")

	acmeCert := &tls.Certificate{
		Certificate: [][]byte{certBytes},
		PrivateKey:  &TheKey,
	}

	hs := tlsalpn01SrvWithCert(t, acmeCert, tls.VersionTLS12)

	va, _ := setup(hs, 0, "", nil, nil)

	_, err = va.validateTLSALPN01(ctx, dnsi("expected"), expectedKeyAuthorization)
	test.AssertError(t, err, "validation should have failed")
	prob := detailedError(err)
	// In go >= 1.19, the TLS client library detects that the certificate has
	// a duplicate extension and terminates the connection itself.
	test.AssertContains(t, prob.Error(), "Error getting validation data")
}

func TestAcceptableExtensions(t *testing.T) {
	requireAcmeAndSAN := []asn1.ObjectIdentifier{
		IdPeAcmeIdentifier,
		IdCeSubjectAltName,
	}

	var err error
	subjectAltName := pkix.Extension{}
	subjectAltName.Id = asn1.ObjectIdentifier{2, 5, 29, 17}
	subjectAltName.Critical = false
	subjectAltName.Value, err = asn1.Marshal([]asn1.RawValue{
		{Tag: 2, Class: 2, Bytes: []byte(`expected`)},
	})
	test.AssertNotError(t, err, "failed to marshal SAN")

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
