package notmain

import (
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/letsencrypt/boulder/test"
)

func TestLoadChain(t *testing.T) {
	// Most of loadChain's logic is implemented in issuance.LoadChain, so this
	// test only covers the construction of the PEM bytes.
	_, chainPEM, err := loadChain([]string{
		"../../test/hierarchy/int-e1.cert.pem",
		"../../test/hierarchy/root-x2-cross.cert.pem",
		"../../test/hierarchy/root-x1.cert.pem",
	})
	test.AssertNotError(t, err, "Should load valid chain")

	// Parse the first certificate in the PEM blob.
	certPEM, rest := pem.Decode(chainPEM)
	test.AssertNotNil(t, certPEM, "Failed to decode chain PEM")
	_, err = x509.ParseCertificate(certPEM.Bytes)
	test.AssertNotError(t, err, "Failed to parse chain PEM")

	// Parse the second certificate in the PEM blob.
	certPEM, rest = pem.Decode(rest)
	test.AssertNotNil(t, certPEM, "Failed to decode chain PEM")
	_, err = x509.ParseCertificate(certPEM.Bytes)
	test.AssertNotError(t, err, "Failed to parse chain PEM")

	// The chain should contain nothing else.
	certPEM, rest = pem.Decode(rest)
	if certPEM != nil || len(rest) != 0 {
		t.Error("Expected chain PEM to contain one cert and nothing else")
	}
}
