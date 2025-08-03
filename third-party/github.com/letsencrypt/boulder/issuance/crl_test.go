package issuance

import (
	"crypto/x509"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/jmhodges/clock"
	"github.com/zmap/zlint/v3/lint"
	"golang.org/x/crypto/cryptobyte"
	cryptobyte_asn1 "golang.org/x/crypto/cryptobyte/asn1"

	"github.com/letsencrypt/boulder/config"
	"github.com/letsencrypt/boulder/crl/idp"
	"github.com/letsencrypt/boulder/test"
)

func TestNewCRLProfile(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		config      CRLProfileConfig
		expected    *CRLProfile
		expectedErr string
	}{
		{
			name:        "validity too long",
			config:      CRLProfileConfig{ValidityInterval: config.Duration{Duration: 30 * 24 * time.Hour}},
			expected:    nil,
			expectedErr: "lifetime cannot be more than 10 days",
		},
		{
			name:        "validity too short",
			config:      CRLProfileConfig{ValidityInterval: config.Duration{Duration: 0}},
			expected:    nil,
			expectedErr: "lifetime must be positive",
		},
		{
			name: "negative backdate",
			config: CRLProfileConfig{
				ValidityInterval: config.Duration{Duration: 7 * 24 * time.Hour},
				MaxBackdate:      config.Duration{Duration: -time.Hour},
			},
			expected:    nil,
			expectedErr: "backdate must be non-negative",
		},
		{
			name: "happy path",
			config: CRLProfileConfig{
				ValidityInterval: config.Duration{Duration: 7 * 24 * time.Hour},
				MaxBackdate:      config.Duration{Duration: time.Hour},
			},
			expected: &CRLProfile{
				validityInterval: 7 * 24 * time.Hour,
				maxBackdate:      time.Hour,
			},
			expectedErr: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			actual, err := NewCRLProfile(tc.config)
			if err != nil {
				if tc.expectedErr == "" {
					t.Errorf("NewCRLProfile expected success but got %q", err)
					return
				}
				test.AssertContains(t, err.Error(), tc.expectedErr)
			} else {
				if tc.expectedErr != "" {
					t.Errorf("NewCRLProfile succeeded but expected error %q", tc.expectedErr)
					return
				}
				test.AssertEquals(t, actual.validityInterval, tc.expected.validityInterval)
				test.AssertEquals(t, actual.maxBackdate, tc.expected.maxBackdate)
				test.AssertNotNil(t, actual.lints, "lint registry should be populated")
			}
		})
	}
}

func TestIssueCRL(t *testing.T) {
	clk := clock.NewFake()
	clk.Set(time.Now())

	issuer, err := newIssuer(defaultIssuerConfig(), issuerCert, issuerSigner, clk)
	test.AssertNotError(t, err, "creating test issuer")

	defaultProfile := CRLProfile{
		validityInterval: 7 * 24 * time.Hour,
		maxBackdate:      1 * time.Hour,
		lints:            lint.GlobalRegistry(),
	}

	defaultRequest := CRLRequest{
		Number:     big.NewInt(123),
		Shard:      100,
		ThisUpdate: clk.Now().Add(-time.Second),
		Entries: []x509.RevocationListEntry{
			{
				SerialNumber:   big.NewInt(987),
				RevocationTime: clk.Now().Add(-24 * time.Hour),
				ReasonCode:     1,
			},
		},
	}

	req := defaultRequest
	req.ThisUpdate = clk.Now().Add(-24 * time.Hour)
	_, err = issuer.IssueCRL(&defaultProfile, &req)
	test.AssertError(t, err, "too old crl issuance should fail")
	test.AssertContains(t, err.Error(), "ThisUpdate is too far in the past")

	req = defaultRequest
	req.ThisUpdate = clk.Now().Add(time.Second)
	_, err = issuer.IssueCRL(&defaultProfile, &req)
	test.AssertError(t, err, "future crl issuance should fail")
	test.AssertContains(t, err.Error(), "ThisUpdate is in the future")

	req = defaultRequest
	req.Entries = append(req.Entries, x509.RevocationListEntry{
		SerialNumber:   big.NewInt(876),
		RevocationTime: clk.Now().Add(-24 * time.Hour),
		ReasonCode:     6,
	})
	_, err = issuer.IssueCRL(&defaultProfile, &req)
	test.AssertError(t, err, "invalid reason code should result in lint failure")
	test.AssertContains(t, err.Error(), "Reason code not included in BR")

	req = defaultRequest
	res, err := issuer.IssueCRL(&defaultProfile, &req)
	test.AssertNotError(t, err, "crl issuance should have succeeded")
	parsedRes, err := x509.ParseRevocationList(res)
	test.AssertNotError(t, err, "parsing test crl")
	test.AssertEquals(t, parsedRes.Issuer.CommonName, issuer.Cert.Subject.CommonName)
	test.AssertDeepEquals(t, parsedRes.Number, big.NewInt(123))
	expectUpdate := req.ThisUpdate.Add(-time.Second).Add(defaultProfile.validityInterval).Truncate(time.Second).UTC()
	test.AssertEquals(t, parsedRes.NextUpdate, expectUpdate)
	test.AssertEquals(t, len(parsedRes.Extensions), 3)
	found, err := revokedCertificatesFieldExists(res)
	test.AssertNotError(t, err, "Should have been able to parse CRL")
	test.Assert(t, found, "Expected the revokedCertificates field to exist")

	idps, err := idp.GetIDPURIs(parsedRes.Extensions)
	test.AssertNotError(t, err, "getting IDP URIs from test CRL")
	test.AssertEquals(t, len(idps), 1)
	test.AssertEquals(t, idps[0], "http://crl-url.example.org/100.crl")

	req = defaultRequest
	crlURLBase := issuer.crlURLBase
	issuer.crlURLBase = ""
	_, err = issuer.IssueCRL(&defaultProfile, &req)
	test.AssertError(t, err, "crl issuance with no IDP should fail")
	test.AssertContains(t, err.Error(), "must contain an issuingDistributionPoint")
	issuer.crlURLBase = crlURLBase

	// A CRL with no entries must not have the revokedCertificates field
	req = defaultRequest
	req.Entries = []x509.RevocationListEntry{}
	res, err = issuer.IssueCRL(&defaultProfile, &req)
	test.AssertNotError(t, err, "issuing crl with no entries")
	parsedRes, err = x509.ParseRevocationList(res)
	test.AssertNotError(t, err, "parsing test crl")
	test.AssertEquals(t, parsedRes.Issuer.CommonName, issuer.Cert.Subject.CommonName)
	test.AssertDeepEquals(t, parsedRes.Number, big.NewInt(123))
	test.AssertEquals(t, len(parsedRes.RevokedCertificateEntries), 0)
	found, err = revokedCertificatesFieldExists(res)
	test.AssertNotError(t, err, "Should have been able to parse CRL")
	test.Assert(t, !found, "Violation of RFC 5280 Section 5.1.2.6")
}

// revokedCertificatesFieldExists is a modified version of
// x509.ParseRevocationList that takes a given sequence of bytes representing a
// CRL and parses away layers until the optional `revokedCertificates` field of
// a TBSCertList is found. It returns a boolean indicating whether the field was
// found or an error if there was an issue processing a CRL.
//
// https://datatracker.ietf.org/doc/html/rfc5280#section-5.1.2.6
//
//	When there are no revoked certificates, the revoked certificates list
//	MUST be absent.
//
// https://datatracker.ietf.org/doc/html/rfc5280#appendix-A.1 page 118
//
//	CertificateList  ::=  SEQUENCE  {
//		tbsCertList          TBSCertList
//	     ..
//	}
//
//	TBSCertList  ::=  SEQUENCE  {
//		..
//		revokedCertificates     SEQUENCE OF SEQUENCE  {
//		..
//		} OPTIONAL,
//	}
func revokedCertificatesFieldExists(der []byte) (bool, error) {
	input := cryptobyte.String(der)

	// Extract the CertificateList
	if !input.ReadASN1(&input, cryptobyte_asn1.SEQUENCE) {
		return false, errors.New("malformed crl")
	}

	var tbs cryptobyte.String
	// Extract the TBSCertList from the CertificateList
	if !input.ReadASN1(&tbs, cryptobyte_asn1.SEQUENCE) {
		return false, errors.New("malformed tbs crl")
	}

	// Skip optional version
	tbs.SkipOptionalASN1(cryptobyte_asn1.INTEGER)

	// Skip the signature
	tbs.SkipASN1(cryptobyte_asn1.SEQUENCE)

	// Skip the issuer
	tbs.SkipASN1(cryptobyte_asn1.SEQUENCE)

	// SkipOptionalASN1 is identical to SkipASN1 except that it also does a
	// peek. We'll handle the non-optional thisUpdate with these double peeks
	// because there's no harm doing so.
	skipTime := func(s *cryptobyte.String) {
		switch {
		case s.PeekASN1Tag(cryptobyte_asn1.UTCTime):
			s.SkipOptionalASN1(cryptobyte_asn1.UTCTime)
		case s.PeekASN1Tag(cryptobyte_asn1.GeneralizedTime):
			s.SkipOptionalASN1(cryptobyte_asn1.GeneralizedTime)
		}
	}

	// Skip thisUpdate
	skipTime(&tbs)

	// Skip optional nextUpdate
	skipTime(&tbs)

	// Finally, the field which we care about: revokedCertificates. This will
	// not trigger on the next field `crlExtensions` because that has
	// context-specific tag [0] and EXPLICIT encoding, not `SEQUENCE` and is
	// therefore a safe place to end this venture.
	if tbs.PeekASN1Tag(cryptobyte_asn1.SEQUENCE) {
		return true, nil
	}

	return false, nil
}
