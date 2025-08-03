package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"

	"github.com/letsencrypt/boulder/test"
)

func TestGenerateOCSPResponse(t *testing.T) {
	kA, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	test.AssertNotError(t, err, "failed to generate test key")
	kB, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	test.AssertNotError(t, err, "failed to generate test key")
	kC, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	test.AssertNotError(t, err, "failed to generate test key")

	template := &x509.Certificate{
		SerialNumber: big.NewInt(9),
		Subject: pkix.Name{
			CommonName: "cn",
		},
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		NotBefore:             time.Time{}.Add(time.Hour * 10),
		NotAfter:              time.Time{}.Add(time.Hour * 20),
	}
	issuerBytes, err := x509.CreateCertificate(rand.Reader, template, template, kA.Public(), kA)
	test.AssertNotError(t, err, "failed to create test issuer")
	issuer, err := x509.ParseCertificate(issuerBytes)
	test.AssertNotError(t, err, "failed to parse test issuer")
	delegatedIssuerBytes, err := x509.CreateCertificate(rand.Reader, template, issuer, kB.Public(), kA)
	test.AssertNotError(t, err, "failed to create test delegated issuer")
	badDelegatedIssuer, err := x509.ParseCertificate(delegatedIssuerBytes)
	test.AssertNotError(t, err, "failed to parse test delegated issuer")
	template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageOCSPSigning}
	delegatedIssuerBytes, err = x509.CreateCertificate(rand.Reader, template, issuer, kB.Public(), kA)
	test.AssertNotError(t, err, "failed to create test delegated issuer")
	goodDelegatedIssuer, err := x509.ParseCertificate(delegatedIssuerBytes)
	test.AssertNotError(t, err, "failed to parse test delegated issuer")
	template.BasicConstraintsValid, template.IsCA = false, false
	certBytes, err := x509.CreateCertificate(rand.Reader, template, issuer, kC.Public(), kA)
	test.AssertNotError(t, err, "failed to create test cert")
	cert, err := x509.ParseCertificate(certBytes)
	test.AssertNotError(t, err, "failed to parse test cert")

	cases := []struct {
		name            string
		issuer          *x509.Certificate
		delegatedIssuer *x509.Certificate
		cert            *x509.Certificate
		thisUpdate      time.Time
		nextUpdate      time.Time
		expectedError   string
	}{
		{
			name:          "invalid signature from issuer on certificate",
			issuer:        &x509.Certificate{},
			cert:          &x509.Certificate{},
			expectedError: "invalid signature on certificate from issuer: x509: cannot verify signature: algorithm unimplemented",
		},
		{
			name:          "nextUpdate before thisUpdate",
			issuer:        issuer,
			cert:          cert,
			thisUpdate:    time.Time{}.Add(time.Hour),
			nextUpdate:    time.Time{},
			expectedError: "thisUpdate must be before nextUpdate",
		},
		{
			name:          "thisUpdate before signer notBefore",
			issuer:        issuer,
			cert:          cert,
			thisUpdate:    time.Time{},
			nextUpdate:    time.Time{}.Add(time.Hour),
			expectedError: "thisUpdate is before signing certificate's notBefore",
		},
		{
			name:          "nextUpdate after signer notAfter",
			issuer:        issuer,
			cert:          cert,
			thisUpdate:    time.Time{}.Add(time.Hour * 11),
			nextUpdate:    time.Time{}.Add(time.Hour * 21),
			expectedError: "nextUpdate is after signing certificate's notAfter",
		},
		{
			name:            "bad delegated issuer signature",
			issuer:          issuer,
			cert:            cert,
			delegatedIssuer: &x509.Certificate{},
			expectedError:   "invalid signature on delegated issuer from issuer: x509: cannot verify signature: algorithm unimplemented",
		},
		{
			name:       "good",
			issuer:     issuer,
			cert:       cert,
			thisUpdate: time.Time{}.Add(time.Hour * 11),
			nextUpdate: time.Time{}.Add(time.Hour * 12),
		},
		{
			name:            "bad delegated issuer without EKU",
			issuer:          issuer,
			cert:            cert,
			delegatedIssuer: badDelegatedIssuer,
			expectedError:   "delegated issuer certificate doesn't contain OCSPSigning extended key usage",
		},
		{
			name:            "good delegated issuer",
			issuer:          issuer,
			cert:            cert,
			delegatedIssuer: goodDelegatedIssuer,
			thisUpdate:      time.Time{}.Add(time.Hour * 11),
			nextUpdate:      time.Time{}.Add(time.Hour * 12),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := generateOCSPResponse(kA, tc.issuer, tc.delegatedIssuer, tc.cert, tc.thisUpdate, tc.nextUpdate, 0)
			if err != nil {
				if tc.expectedError != "" && tc.expectedError != err.Error() {
					t.Errorf("unexpected error: got %q, want %q", err.Error(), tc.expectedError)
				} else if tc.expectedError == "" {
					t.Errorf("unexpected error: %s", err)
				}
			} else if tc.expectedError != "" {
				t.Errorf("expected error: %s", tc.expectedError)
			}
		})
	}
}
