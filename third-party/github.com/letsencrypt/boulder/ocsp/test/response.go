package ocsp_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"

	"golang.org/x/crypto/ocsp"
)

// FakeResponse signs and then parses an OCSP response, using fields from the input
// template. To do so, it generates a new signing key and makes an issuer certificate.
func FakeResponse(template ocsp.Response) (*ocsp.Response, *x509.Certificate, error) {
	// Make a fake CA to sign OCSP with
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	certTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1337),
		BasicConstraintsValid: true,
		IsCA:                  true,
		Subject:               pkix.Name{CommonName: "test CA"},
	}
	issuerBytes, err := x509.CreateCertificate(rand.Reader, certTemplate, certTemplate, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}

	issuer, err := x509.ParseCertificate(issuerBytes)
	if err != nil {
		return nil, nil, err
	}

	respBytes, err := ocsp.CreateResponse(issuer, issuer, template, key)
	if err != nil {
		return nil, nil, err
	}

	response, err := ocsp.ParseResponse(respBytes, issuer)
	if err != nil {
		return nil, nil, err
	}
	return response, issuer, nil
}
