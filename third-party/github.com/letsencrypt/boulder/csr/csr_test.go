package csr

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
	"net"
	"strings"
	"testing"

	"github.com/letsencrypt/boulder/core"
	berrors "github.com/letsencrypt/boulder/errors"
	"github.com/letsencrypt/boulder/features"
	"github.com/letsencrypt/boulder/goodkey"
	"github.com/letsencrypt/boulder/identifier"
	"github.com/letsencrypt/boulder/test"
)

type mockPA struct{}

func (pa *mockPA) ChallengesFor(identifier identifier.ACMEIdentifier) (challenges []core.Challenge, err error) {
	return
}

func (pa *mockPA) WillingToIssue(domains []string) error {
	for _, domain := range domains {
		if domain == "bad-name.com" || domain == "other-bad-name.com" {
			return errors.New("policy forbids issuing for identifier")
		}
	}
	return nil
}

func (pa *mockPA) ChallengeTypeEnabled(t core.AcmeChallenge) bool {
	return true
}

func (pa *mockPA) CheckAuthz(a *core.Authorization) error {
	return nil
}

func TestVerifyCSR(t *testing.T) {
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	test.AssertNotError(t, err, "error generating test key")
	signedReqBytes, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{PublicKey: private.PublicKey, SignatureAlgorithm: x509.SHA256WithRSA}, private)
	test.AssertNotError(t, err, "error generating test CSR")
	signedReq, err := x509.ParseCertificateRequest(signedReqBytes)
	test.AssertNotError(t, err, "error parsing test CSR")
	brokenSignedReq := new(x509.CertificateRequest)
	*brokenSignedReq = *signedReq
	brokenSignedReq.Signature = []byte{1, 1, 1, 1}
	signedReqWithHosts := new(x509.CertificateRequest)
	*signedReqWithHosts = *signedReq
	signedReqWithHosts.DNSNames = []string{"a.com", "b.com"}
	signedReqWithLongCN := new(x509.CertificateRequest)
	*signedReqWithLongCN = *signedReq
	signedReqWithLongCN.Subject.CommonName = strings.Repeat("a", maxCNLength+1)
	signedReqWithBadNames := new(x509.CertificateRequest)
	*signedReqWithBadNames = *signedReq
	signedReqWithBadNames.DNSNames = []string{"bad-name.com", "other-bad-name.com"}
	signedReqWithEmailAddress := new(x509.CertificateRequest)
	*signedReqWithEmailAddress = *signedReq
	signedReqWithEmailAddress.EmailAddresses = []string{"foo@bar.com"}
	signedReqWithIPAddress := new(x509.CertificateRequest)
	*signedReqWithIPAddress = *signedReq
	signedReqWithIPAddress.IPAddresses = []net.IP{net.IPv4(1, 2, 3, 4)}
	signedReqWithAllLongSANs := new(x509.CertificateRequest)
	*signedReqWithAllLongSANs = *signedReq
	signedReqWithAllLongSANs.DNSNames = []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.com"}

	keyPolicy, err := goodkey.NewPolicy(nil, nil)
	test.AssertNotError(t, err, "creating test keypolicy")

	cases := []struct {
		csr           *x509.CertificateRequest
		maxNames      int
		pa            core.PolicyAuthority
		expectedError error
	}{
		{
			&x509.CertificateRequest{},
			100,
			&mockPA{},
			invalidPubKey,
		},
		{
			&x509.CertificateRequest{PublicKey: &private.PublicKey},
			100,
			&mockPA{},
			unsupportedSigAlg,
		},
		{
			brokenSignedReq,
			100,
			&mockPA{},
			invalidSig,
		},
		{
			signedReq,
			100,
			&mockPA{},
			invalidNoDNS,
		},
		{
			signedReqWithLongCN,
			100,
			&mockPA{},
			berrors.BadCSRError("CN was longer than %d bytes", maxCNLength),
		},
		{
			signedReqWithHosts,
			1,
			&mockPA{},
			berrors.BadCSRError("CSR contains more than 1 DNS names"),
		},
		{
			signedReqWithBadNames,
			100,
			&mockPA{},
			errors.New("policy forbids issuing for identifier"),
		},
		{
			signedReqWithEmailAddress,
			100,
			&mockPA{},
			invalidEmailPresent,
		},
		{
			signedReqWithIPAddress,
			100,
			&mockPA{},
			invalidIPPresent,
		},
		{
			signedReqWithAllLongSANs,
			100,
			&mockPA{},
			nil,
		},
	}

	for _, c := range cases {
		err := VerifyCSR(context.Background(), c.csr, c.maxNames, &keyPolicy, c.pa)
		test.AssertDeepEquals(t, c.expectedError, err)
	}
}

func TestNamesFromCSR(t *testing.T) {
	tooLongString := strings.Repeat("a", maxCNLength+1)

	cases := []struct {
		name          string
		csr           *x509.CertificateRequest
		expectedCN    string
		expectedNames []string
	}{
		{
			"no explicit CN",
			&x509.CertificateRequest{DNSNames: []string{"a.com"}},
			"a.com",
			[]string{"a.com"},
		},
		{
			"explicit uppercase CN",
			&x509.CertificateRequest{Subject: pkix.Name{CommonName: "A.com"}, DNSNames: []string{"a.com"}},
			"a.com",
			[]string{"a.com"},
		},
		{
			"no explicit CN, uppercase SAN",
			&x509.CertificateRequest{DNSNames: []string{"A.com"}},
			"a.com",
			[]string{"a.com"},
		},
		{
			"duplicate SANs",
			&x509.CertificateRequest{DNSNames: []string{"b.com", "b.com", "a.com", "a.com"}},
			"b.com",
			[]string{"a.com", "b.com"},
		},
		{
			"explicit CN not found in SANs",
			&x509.CertificateRequest{Subject: pkix.Name{CommonName: "a.com"}, DNSNames: []string{"b.com"}},
			"a.com",
			[]string{"a.com", "b.com"},
		},
		{
			"no explicit CN, too long leading SANs",
			&x509.CertificateRequest{DNSNames: []string{
				tooLongString + ".a.com",
				tooLongString + ".b.com",
				"a.com",
				"b.com",
			}},
			"a.com",
			[]string{"a.com", tooLongString + ".a.com", tooLongString + ".b.com", "b.com"},
		},
		{
			"explicit CN, too long leading SANs",
			&x509.CertificateRequest{
				Subject: pkix.Name{CommonName: "A.com"},
				DNSNames: []string{
					tooLongString + ".a.com",
					tooLongString + ".b.com",
					"a.com",
					"b.com",
				}},
			"a.com",
			[]string{"a.com", tooLongString + ".a.com", tooLongString + ".b.com", "b.com"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			names := NamesFromCSR(tc.csr)
			test.AssertEquals(t, names.CN, tc.expectedCN)
			test.AssertDeepEquals(t, names.SANs, tc.expectedNames)
		})
	}
}

func TestSHA1Deprecation(t *testing.T) {
	features.Reset()

	keyPolicy, err := goodkey.NewPolicy(nil, nil)
	test.AssertNotError(t, err, "creating test keypolicy")

	private, err := rsa.GenerateKey(rand.Reader, 2048)
	test.AssertNotError(t, err, "error generating test key")

	makeAndVerifyCsr := func(alg x509.SignatureAlgorithm) error {
		csrBytes, err := x509.CreateCertificateRequest(rand.Reader,
			&x509.CertificateRequest{
				DNSNames:           []string{"example.com"},
				SignatureAlgorithm: alg,
				PublicKey:          &private.PublicKey,
			}, private)
		test.AssertNotError(t, err, "creating test CSR")

		csr, err := x509.ParseCertificateRequest(csrBytes)
		test.AssertNotError(t, err, "parsing test CSR")

		return VerifyCSR(context.Background(), csr, 100, &keyPolicy, &mockPA{})
	}

	err = makeAndVerifyCsr(x509.SHA256WithRSA)
	test.AssertNotError(t, err, "SHA256 CSR should verify")

	err = makeAndVerifyCsr(x509.SHA1WithRSA)
	test.AssertError(t, err, "SHA1 CSR should not verify")
}

func TestDuplicateExtensionRejection(t *testing.T) {
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	test.AssertNotError(t, err, "error generating test key")

	csrBytes, err := x509.CreateCertificateRequest(rand.Reader,
		&x509.CertificateRequest{
			DNSNames:           []string{"example.com"},
			SignatureAlgorithm: x509.SHA256WithRSA,
			PublicKey:          &private.PublicKey,
			ExtraExtensions: []pkix.Extension{
				{Id: asn1.ObjectIdentifier{2, 5, 29, 1}, Value: []byte("hello")},
				{Id: asn1.ObjectIdentifier{2, 5, 29, 1}, Value: []byte("world")},
			},
		}, private)
	test.AssertNotError(t, err, "creating test CSR")

	_, err = x509.ParseCertificateRequest(csrBytes)
	test.AssertError(t, err, "CSR with duplicate extension OID should fail to parse")
}
