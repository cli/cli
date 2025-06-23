package test

import (
	"encoding/pem"
	"os"
	"testing"

	"github.com/zmap/zcrypto/x509"

	"github.com/letsencrypt/boulder/test"
)

func LoadPEMCRL(t *testing.T, filename string) *x509.RevocationList {
	t.Helper()
	file, err := os.ReadFile(filename)
	test.AssertNotError(t, err, "reading CRL file")
	block, rest := pem.Decode(file)
	test.AssertEquals(t, block.Type, "X509 CRL")
	test.AssertEquals(t, len(rest), 0)
	crl, err := x509.ParseRevocationList(block.Bytes)
	test.AssertNotError(t, err, "parsing CRL bytes")
	return crl
}
