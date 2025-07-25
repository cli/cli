package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"math/big"
	"testing"
	"time"

	"github.com/miekg/pkcs11"

	"github.com/letsencrypt/boulder/pkcs11helpers"
	"github.com/letsencrypt/boulder/test"
)

// samplePubkey returns a slice of bytes containing an encoded
// SubjectPublicKeyInfo for an example public key.
func samplePubkey() []byte {
	pubKey, err := hex.DecodeString("3059301306072a8648ce3d020106082a8648ce3d03010703420004b06745ef0375c9c54057098f077964e18d3bed0aacd54545b16eab8c539b5768cc1cea93ba56af1e22a7a01c33048c8885ed17c9c55ede70649b707072689f5e")
	if err != nil {
		panic(err)
	}
	return pubKey
}

func realRand(_ pkcs11.SessionHandle, length int) ([]byte, error) {
	r := make([]byte, length)
	_, err := rand.Read(r)
	return r, err
}

func TestParseOID(t *testing.T) {
	_, err := parseOID("")
	test.AssertError(t, err, "parseOID accepted an empty OID")
	_, err = parseOID("a.b.c")
	test.AssertError(t, err, "parseOID accepted an OID containing non-ints")
	_, err = parseOID("1.0.2")
	test.AssertError(t, err, "parseOID accepted an OID containing zero")
	oid, err := parseOID("1.2.3")
	test.AssertNotError(t, err, "parseOID failed with a valid OID")
	test.Assert(t, oid.Equal(asn1.ObjectIdentifier{1, 2, 3}), "parseOID returned incorrect OID")
}

func TestMakeSubject(t *testing.T) {
	profile := &certProfile{
		CommonName:   "common name",
		Organization: "organization",
		Country:      "country",
	}
	expectedSubject := pkix.Name{
		CommonName:   "common name",
		Organization: []string{"organization"},
		Country:      []string{"country"},
	}
	test.AssertDeepEquals(t, profile.Subject(), expectedSubject)
}

func TestMakeTemplateRoot(t *testing.T) {
	s, ctx := pkcs11helpers.NewSessionWithMock()
	profile := &certProfile{}
	randReader := newRandReader(s)
	pubKey := samplePubkey()
	ctx.GenerateRandomFunc = realRand

	profile.NotBefore = "1234"
	_, err := makeTemplate(randReader, profile, pubKey, nil, rootCert)
	test.AssertError(t, err, "makeTemplate didn't fail with invalid not before")

	profile.NotBefore = "2018-05-18 11:31:00"
	profile.NotAfter = "1234"
	_, err = makeTemplate(randReader, profile, pubKey, nil, rootCert)
	test.AssertError(t, err, "makeTemplate didn't fail with invalid not after")

	profile.NotAfter = "2018-05-18 11:31:00"
	profile.SignatureAlgorithm = "nope"
	_, err = makeTemplate(randReader, profile, pubKey, nil, rootCert)
	test.AssertError(t, err, "makeTemplate didn't fail with invalid signature algorithm")

	profile.SignatureAlgorithm = "SHA256WithRSA"
	ctx.GenerateRandomFunc = func(pkcs11.SessionHandle, int) ([]byte, error) {
		return nil, errors.New("bad")
	}
	_, err = makeTemplate(randReader, profile, pubKey, nil, rootCert)
	test.AssertError(t, err, "makeTemplate didn't fail when GenerateRandom failed")

	ctx.GenerateRandomFunc = realRand

	_, err = makeTemplate(randReader, profile, pubKey, nil, rootCert)
	test.AssertError(t, err, "makeTemplate didn't fail with empty key usages")

	profile.KeyUsages = []string{"asd"}
	_, err = makeTemplate(randReader, profile, pubKey, nil, rootCert)
	test.AssertError(t, err, "makeTemplate didn't fail with invalid key usages")

	profile.KeyUsages = []string{"Digital Signature", "CRL Sign"}
	profile.Policies = []policyInfoConfig{{}}
	_, err = makeTemplate(randReader, profile, pubKey, nil, rootCert)
	test.AssertError(t, err, "makeTemplate didn't fail with invalid (empty) policy OID")

	profile.Policies = []policyInfoConfig{{OID: "1.2.3"}, {OID: "1.2.3.4"}}
	profile.CommonName = "common name"
	profile.Organization = "organization"
	profile.Country = "country"
	profile.OCSPURL = "ocsp"
	profile.CRLURL = "crl"
	profile.IssuerURL = "issuer"
	cert, err := makeTemplate(randReader, profile, pubKey, nil, rootCert)
	test.AssertNotError(t, err, "makeTemplate failed when everything worked as expected")
	test.AssertEquals(t, cert.Subject.CommonName, profile.CommonName)
	test.AssertEquals(t, len(cert.Subject.Organization), 1)
	test.AssertEquals(t, cert.Subject.Organization[0], profile.Organization)
	test.AssertEquals(t, len(cert.Subject.Country), 1)
	test.AssertEquals(t, cert.Subject.Country[0], profile.Country)
	test.AssertEquals(t, len(cert.OCSPServer), 1)
	test.AssertEquals(t, cert.OCSPServer[0], profile.OCSPURL)
	test.AssertEquals(t, len(cert.CRLDistributionPoints), 1)
	test.AssertEquals(t, cert.CRLDistributionPoints[0], profile.CRLURL)
	test.AssertEquals(t, len(cert.IssuingCertificateURL), 1)
	test.AssertEquals(t, cert.IssuingCertificateURL[0], profile.IssuerURL)
	test.AssertEquals(t, cert.KeyUsage, x509.KeyUsageDigitalSignature|x509.KeyUsageCRLSign)
	test.AssertEquals(t, len(cert.Policies), 2)
	test.AssertEquals(t, len(cert.ExtKeyUsage), 0)

	cert, err = makeTemplate(randReader, profile, pubKey, nil, intermediateCert)
	test.AssertNotError(t, err, "makeTemplate failed when everything worked as expected")
	test.Assert(t, cert.MaxPathLenZero, "MaxPathLenZero not set in intermediate template")
	test.AssertEquals(t, len(cert.ExtKeyUsage), 1)
	test.AssertEquals(t, cert.ExtKeyUsage[0], x509.ExtKeyUsageServerAuth)
}

func TestMakeTemplateRestrictedCrossCertificate(t *testing.T) {
	s, ctx := pkcs11helpers.NewSessionWithMock()
	ctx.GenerateRandomFunc = realRand
	randReader := newRandReader(s)
	pubKey := samplePubkey()
	profile := &certProfile{
		SignatureAlgorithm: "SHA256WithRSA",
		CommonName:         "common name",
		Organization:       "organization",
		Country:            "country",
		KeyUsages:          []string{"Digital Signature", "CRL Sign"},
		OCSPURL:            "ocsp",
		CRLURL:             "crl",
		IssuerURL:          "issuer",
		NotAfter:           "2020-10-10 11:31:00",
		NotBefore:          "2020-10-10 11:31:00",
	}

	tbcsCert := x509.Certificate{
		SerialNumber: big.NewInt(666),
		Subject: pkix.Name{
			Organization: []string{"While Eek Ayote"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	cert, err := makeTemplate(randReader, profile, pubKey, &tbcsCert, crossCert)
	test.AssertNotError(t, err, "makeTemplate failed when everything worked as expected")
	test.Assert(t, !cert.MaxPathLenZero, "MaxPathLenZero was set in cross-sign")
	test.AssertEquals(t, len(cert.ExtKeyUsage), 1)
	test.AssertEquals(t, cert.ExtKeyUsage[0], x509.ExtKeyUsageServerAuth)
}

func TestMakeTemplateOCSP(t *testing.T) {
	s, ctx := pkcs11helpers.NewSessionWithMock()
	ctx.GenerateRandomFunc = realRand
	randReader := newRandReader(s)
	profile := &certProfile{
		SignatureAlgorithm: "SHA256WithRSA",
		CommonName:         "common name",
		Organization:       "organization",
		Country:            "country",
		OCSPURL:            "ocsp",
		CRLURL:             "crl",
		IssuerURL:          "issuer",
		NotAfter:           "2018-05-18 11:31:00",
		NotBefore:          "2018-05-18 11:31:00",
	}
	pubKey := samplePubkey()

	cert, err := makeTemplate(randReader, profile, pubKey, nil, ocspCert)
	test.AssertNotError(t, err, "makeTemplate failed")

	test.Assert(t, !cert.IsCA, "IsCA is set")
	// Check KU is only KeyUsageDigitalSignature
	test.AssertEquals(t, cert.KeyUsage, x509.KeyUsageDigitalSignature)
	// Check there is a single EKU with id-kp-OCSPSigning
	test.AssertEquals(t, len(cert.ExtKeyUsage), 1)
	test.AssertEquals(t, cert.ExtKeyUsage[0], x509.ExtKeyUsageOCSPSigning)
	// Check ExtraExtensions contains a single id-pkix-ocsp-nocheck
	hasExt := false
	asnNULL := []byte{5, 0}
	for _, ext := range cert.ExtraExtensions {
		if ext.Id.Equal(oidOCSPNoCheck) {
			if hasExt {
				t.Error("template contains multiple id-pkix-ocsp-nocheck extensions")
			}
			hasExt = true
			if !bytes.Equal(ext.Value, asnNULL) {
				t.Errorf("id-pkix-ocsp-nocheck has unexpected content: want %x, got %x", asnNULL, ext.Value)
			}
		}
	}
	test.Assert(t, hasExt, "template doesn't contain id-pkix-ocsp-nocheck extensions")
}

func TestMakeTemplateCRL(t *testing.T) {
	s, ctx := pkcs11helpers.NewSessionWithMock()
	ctx.GenerateRandomFunc = realRand
	randReader := newRandReader(s)
	profile := &certProfile{
		SignatureAlgorithm: "SHA256WithRSA",
		CommonName:         "common name",
		Organization:       "organization",
		Country:            "country",
		OCSPURL:            "ocsp",
		CRLURL:             "crl",
		IssuerURL:          "issuer",
		NotAfter:           "2018-05-18 11:31:00",
		NotBefore:          "2018-05-18 11:31:00",
	}
	pubKey := samplePubkey()

	cert, err := makeTemplate(randReader, profile, pubKey, nil, crlCert)
	test.AssertNotError(t, err, "makeTemplate failed")

	test.Assert(t, !cert.IsCA, "IsCA is set")
	test.AssertEquals(t, cert.KeyUsage, x509.KeyUsageCRLSign)
}

func TestVerifyProfile(t *testing.T) {
	for _, tc := range []struct {
		profile     certProfile
		certType    []certType
		expectedErr string
	}{
		{
			profile:     certProfile{},
			certType:    []certType{intermediateCert, crossCert},
			expectedErr: "not-before is required",
		},
		{
			profile: certProfile{
				NotBefore: "a",
			},
			certType:    []certType{intermediateCert, crossCert},
			expectedErr: "not-after is required",
		},
		{
			profile: certProfile{
				NotBefore: "a",
				NotAfter:  "b",
			},
			certType:    []certType{intermediateCert, crossCert},
			expectedErr: "signature-algorithm is required",
		},
		{
			profile: certProfile{
				NotBefore:          "a",
				NotAfter:           "b",
				SignatureAlgorithm: "c",
			},
			certType:    []certType{intermediateCert, crossCert},
			expectedErr: "common-name is required",
		},
		{
			profile: certProfile{
				NotBefore:          "a",
				NotAfter:           "b",
				SignatureAlgorithm: "c",
				CommonName:         "d",
			},
			certType:    []certType{intermediateCert, crossCert},
			expectedErr: "organization is required",
		},
		{
			profile: certProfile{
				NotBefore:          "a",
				NotAfter:           "b",
				SignatureAlgorithm: "c",
				CommonName:         "d",
				Organization:       "e",
			},
			certType:    []certType{intermediateCert, crossCert},
			expectedErr: "country is required",
		},
		{
			profile: certProfile{
				NotBefore:          "a",
				NotAfter:           "b",
				SignatureAlgorithm: "c",
				CommonName:         "d",
				Organization:       "e",
				Country:            "f",
				OCSPURL:            "g",
			},
			certType:    []certType{intermediateCert, crossCert},
			expectedErr: "crl-url is required for subordinate CAs",
		},
		{
			profile: certProfile{
				NotBefore:          "a",
				NotAfter:           "b",
				SignatureAlgorithm: "c",
				CommonName:         "d",
				Organization:       "e",
				Country:            "f",
				OCSPURL:            "g",
				CRLURL:             "h",
			},
			certType:    []certType{intermediateCert, crossCert},
			expectedErr: "issuer-url is required for subordinate CAs",
		},
		{
			profile: certProfile{
				NotBefore:          "a",
				NotAfter:           "b",
				SignatureAlgorithm: "c",
				CommonName:         "d",
				Organization:       "e",
				Country:            "f",
				OCSPURL:            "g",
				CRLURL:             "h",
				IssuerURL:          "i",
			},
			certType:    []certType{intermediateCert, crossCert},
			expectedErr: "policy should be exactly BRs domain-validated for subordinate CAs",
		},
		{
			profile: certProfile{
				NotBefore:          "a",
				NotAfter:           "b",
				SignatureAlgorithm: "c",
				CommonName:         "d",
				Organization:       "e",
				Country:            "f",
				OCSPURL:            "g",
				CRLURL:             "h",
				IssuerURL:          "i",
				Policies:           []policyInfoConfig{{OID: "1.2.3"}, {OID: "4.5.6"}},
			},
			certType:    []certType{intermediateCert, crossCert},
			expectedErr: "policy should be exactly BRs domain-validated for subordinate CAs",
		},
		{
			profile: certProfile{
				NotBefore:          "a",
				NotAfter:           "b",
				SignatureAlgorithm: "c",
				CommonName:         "d",
				Organization:       "e",
				Country:            "f",
			},
			certType: []certType{rootCert},
		},
		{
			profile: certProfile{
				NotBefore:          "a",
				NotAfter:           "b",
				SignatureAlgorithm: "c",
				CommonName:         "d",
				Organization:       "e",
				Country:            "f",
				IssuerURL:          "g",
				KeyUsages:          []string{"j"},
			},
			certType:    []certType{ocspCert},
			expectedErr: "key-usages cannot be set for a delegated signer",
		},
		{
			profile: certProfile{
				NotBefore:          "a",
				NotAfter:           "b",
				SignatureAlgorithm: "c",
				CommonName:         "d",
				Organization:       "e",
				Country:            "f",
				IssuerURL:          "g",
				CRLURL:             "i",
			},
			certType:    []certType{ocspCert},
			expectedErr: "crl-url cannot be set for a delegated signer",
		},
		{
			profile: certProfile{
				NotBefore:          "a",
				NotAfter:           "b",
				SignatureAlgorithm: "c",
				CommonName:         "d",
				Organization:       "e",
				Country:            "f",
				IssuerURL:          "g",
				OCSPURL:            "h",
			},
			certType:    []certType{ocspCert},
			expectedErr: "ocsp-url cannot be set for a delegated signer",
		},
		{
			profile: certProfile{
				NotBefore:          "a",
				NotAfter:           "b",
				SignatureAlgorithm: "c",
				CommonName:         "d",
				Organization:       "e",
				Country:            "f",
				IssuerURL:          "g",
			},
			certType: []certType{ocspCert},
		},
		{
			profile: certProfile{
				NotBefore:          "a",
				NotAfter:           "b",
				SignatureAlgorithm: "c",
				CommonName:         "d",
				Organization:       "e",
				Country:            "f",
				IssuerURL:          "g",
				KeyUsages:          []string{"j"},
			},
			certType:    []certType{crlCert},
			expectedErr: "key-usages cannot be set for a delegated signer",
		},
		{
			profile: certProfile{
				NotBefore:          "a",
				NotAfter:           "b",
				SignatureAlgorithm: "c",
				CommonName:         "d",
				Organization:       "e",
				Country:            "f",
				IssuerURL:          "g",
				CRLURL:             "i",
			},
			certType:    []certType{crlCert},
			expectedErr: "crl-url cannot be set for a delegated signer",
		},
		{
			profile: certProfile{
				NotBefore:          "a",
				NotAfter:           "b",
				SignatureAlgorithm: "c",
				CommonName:         "d",
				Organization:       "e",
				Country:            "f",
				IssuerURL:          "g",
				OCSPURL:            "h",
			},
			certType:    []certType{crlCert},
			expectedErr: "ocsp-url cannot be set for a delegated signer",
		},
		{
			profile: certProfile{
				NotBefore:          "a",
				NotAfter:           "b",
				SignatureAlgorithm: "c",
				CommonName:         "d",
				Organization:       "e",
				Country:            "f",
				IssuerURL:          "g",
			},
			certType: []certType{crlCert},
		},
		{
			profile: certProfile{
				NotBefore: "a",
			},
			certType:    []certType{requestCert},
			expectedErr: "not-before cannot be set for a CSR",
		},
		{
			profile: certProfile{
				NotAfter: "a",
			},
			certType:    []certType{requestCert},
			expectedErr: "not-after cannot be set for a CSR",
		},
		{
			profile: certProfile{
				SignatureAlgorithm: "a",
			},
			certType:    []certType{requestCert},
			expectedErr: "signature-algorithm cannot be set for a CSR",
		},
		{
			profile: certProfile{
				OCSPURL: "a",
			},
			certType:    []certType{requestCert},
			expectedErr: "ocsp-url cannot be set for a CSR",
		},
		{
			profile: certProfile{
				CRLURL: "a",
			},
			certType:    []certType{requestCert},
			expectedErr: "crl-url cannot be set for a CSR",
		},
		{
			profile: certProfile{
				IssuerURL: "a",
			},
			certType:    []certType{requestCert},
			expectedErr: "issuer-url cannot be set for a CSR",
		},
		{
			profile: certProfile{
				Policies: []policyInfoConfig{{OID: "1.2.3"}},
			},
			certType:    []certType{requestCert},
			expectedErr: "policies cannot be set for a CSR",
		},
		{
			profile: certProfile{
				KeyUsages: []string{"a"},
			},
			certType:    []certType{requestCert},
			expectedErr: "key-usages cannot be set for a CSR",
		},
	} {
		for _, ct := range tc.certType {
			err := tc.profile.verifyProfile(ct)
			if err != nil {
				if tc.expectedErr != err.Error() {
					t.Fatalf("Expected %q, got %q", tc.expectedErr, err.Error())
				}
			} else if tc.expectedErr != "" {
				t.Fatalf("verifyProfile didn't fail, expected %q", tc.expectedErr)
			}
		}
	}
}

func TestGenerateCSR(t *testing.T) {
	profile := &certProfile{
		CommonName:   "common name",
		Organization: "organization",
		Country:      "country",
	}

	signer, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	test.AssertNotError(t, err, "failed to generate test key")

	csrBytes, err := generateCSR(profile, &wrappedSigner{signer})
	test.AssertNotError(t, err, "failed to generate CSR")

	csr, err := x509.ParseCertificateRequest(csrBytes)
	test.AssertNotError(t, err, "failed to parse CSR")
	test.AssertNotError(t, csr.CheckSignature(), "CSR signature check failed")
	test.AssertEquals(t, len(csr.Extensions), 0)

	test.AssertEquals(t, csr.Subject.String(), fmt.Sprintf("CN=%s,O=%s,C=%s",
		profile.CommonName, profile.Organization, profile.Country))
}

func TestLoadCert(t *testing.T) {
	_, err := loadCert("../../test/hierarchy/int-e1.cert.pem")
	test.AssertNotError(t, err, "should not have errored")

	_, err = loadCert("/path/that/will/not/ever/exist/ever")
	test.AssertError(t, err, "should have failed opening certificate at non-existent path")
	test.AssertErrorIs(t, err, fs.ErrNotExist)

	_, err = loadCert("../../test/hierarchy/int-e1.key.pem")
	test.AssertError(t, err, "should have failed when trying to parse a private key")
}

func TestGenerateSKID(t *testing.T) {
	sha256skid, err := generateSKID(samplePubkey())
	test.AssertNotError(t, err, "Error generating SKID")
	test.AssertEquals(t, len(sha256skid), 20)
	test.AssertEquals(t, cap(sha256skid), 20)
}
