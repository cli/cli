package issuance

import (
	"crypto"
	"crypto/dsa"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"

	ct "github.com/google/certificate-transparency-go"
	"github.com/jmhodges/clock"

	"github.com/letsencrypt/boulder/config"
	"github.com/letsencrypt/boulder/ctpolicy/loglist"
	"github.com/letsencrypt/boulder/linter"
	"github.com/letsencrypt/boulder/test"
)

var (
	goodSKID = []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
)

func defaultProfile() *Profile {
	p, _ := NewProfile(defaultProfileConfig())
	return p
}

func TestGenerateValidity(t *testing.T) {
	fc := clock.NewFake()
	fc.Set(time.Date(2015, time.June, 04, 11, 04, 38, 0, time.UTC))

	tests := []struct {
		name      string
		backdate  time.Duration
		validity  time.Duration
		notBefore time.Time
		notAfter  time.Time
	}{
		{
			name:      "normal usage",
			backdate:  time.Hour, // 90% of one hour is 54 minutes
			validity:  7 * 24 * time.Hour,
			notBefore: time.Date(2015, time.June, 04, 10, 10, 38, 0, time.UTC),
			notAfter:  time.Date(2015, time.June, 11, 10, 10, 37, 0, time.UTC),
		},
		{
			name:      "zero backdate",
			backdate:  0,
			validity:  7 * 24 * time.Hour,
			notBefore: time.Date(2015, time.June, 04, 11, 04, 38, 0, time.UTC),
			notAfter:  time.Date(2015, time.June, 11, 11, 04, 37, 0, time.UTC),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := Profile{maxBackdate: tc.backdate, maxValidity: tc.validity}
			notBefore, notAfter := p.GenerateValidity(fc.Now())
			test.AssertEquals(t, notBefore, tc.notBefore)
			test.AssertEquals(t, notAfter, tc.notAfter)
		})
	}
}

func TestCRLURL(t *testing.T) {
	issuer, err := newIssuer(defaultIssuerConfig(), issuerCert, issuerSigner, clock.NewFake())
	if err != nil {
		t.Fatalf("newIssuer: %s", err)
	}
	url := issuer.crlURL(4928)
	want := "http://crl-url.example.org/4928.crl"
	if url != want {
		t.Errorf("crlURL(4928)=%s, want %s", url, want)
	}
}

func TestRequestValid(t *testing.T) {
	fc := clock.NewFake()
	fc.Add(time.Hour * 24)

	tests := []struct {
		name          string
		issuer        *Issuer
		profile       *Profile
		request       *IssuanceRequest
		expectedError string
	}{
		{
			name:          "unsupported key type",
			issuer:        &Issuer{},
			profile:       &Profile{},
			request:       &IssuanceRequest{PublicKey: MarshalablePublicKey{&dsa.PublicKey{}}},
			expectedError: "unsupported public key type",
		},
		{
			name:          "inactive (rsa)",
			issuer:        &Issuer{},
			profile:       &Profile{},
			request:       &IssuanceRequest{PublicKey: MarshalablePublicKey{&rsa.PublicKey{}}},
			expectedError: "inactive issuer cannot issue precert",
		},
		{
			name:          "inactive (ecdsa)",
			issuer:        &Issuer{},
			profile:       &Profile{},
			request:       &IssuanceRequest{PublicKey: MarshalablePublicKey{&ecdsa.PublicKey{}}},
			expectedError: "inactive issuer cannot issue precert",
		},
		{
			name: "skid too short",
			issuer: &Issuer{
				active: true,
			},
			profile: &Profile{},
			request: &IssuanceRequest{
				PublicKey:    MarshalablePublicKey{&ecdsa.PublicKey{}},
				SubjectKeyId: []byte{0, 1, 2, 3, 4},
			},
			expectedError: "unexpected subject key ID length",
		},
		{
			name: "both sct list and ct poison provided",
			issuer: &Issuer{
				active: true,
			},
			profile: &Profile{},
			request: &IssuanceRequest{
				PublicKey:       MarshalablePublicKey{&ecdsa.PublicKey{}},
				SubjectKeyId:    goodSKID,
				IncludeCTPoison: true,
				sctList:         []ct.SignedCertificateTimestamp{},
			},
			expectedError: "cannot include both ct poison and sct list extensions",
		},
		{
			name: "negative validity",
			issuer: &Issuer{
				active: true,
			},
			profile: &Profile{},
			request: &IssuanceRequest{
				PublicKey:    MarshalablePublicKey{&ecdsa.PublicKey{}},
				SubjectKeyId: goodSKID,
				NotBefore:    fc.Now().Add(time.Hour),
				NotAfter:     fc.Now(),
			},
			expectedError: "NotAfter must be after NotBefore",
		},
		{
			name: "validity larger than max",
			issuer: &Issuer{
				active: true,
			},
			profile: &Profile{
				maxValidity: time.Minute,
			},
			request: &IssuanceRequest{
				PublicKey:    MarshalablePublicKey{&ecdsa.PublicKey{}},
				SubjectKeyId: goodSKID,
				NotBefore:    fc.Now(),
				NotAfter:     fc.Now().Add(time.Hour - time.Second),
			},
			expectedError: "validity period is more than the maximum allowed period (1h0m0s>1m0s)",
		},
		{
			name: "validity larger than max due to inclusivity",
			issuer: &Issuer{
				active: true,
			},
			profile: &Profile{
				maxValidity: time.Hour,
			},
			request: &IssuanceRequest{
				PublicKey:    MarshalablePublicKey{&ecdsa.PublicKey{}},
				SubjectKeyId: goodSKID,
				NotBefore:    fc.Now(),
				NotAfter:     fc.Now().Add(time.Hour),
			},
			expectedError: "validity period is more than the maximum allowed period (1h0m1s>1h0m0s)",
		},
		{
			name: "validity backdated more than max",
			issuer: &Issuer{
				active: true,
			},
			profile: &Profile{
				maxValidity: time.Hour * 2,
				maxBackdate: time.Hour,
			},
			request: &IssuanceRequest{
				PublicKey:    MarshalablePublicKey{&ecdsa.PublicKey{}},
				SubjectKeyId: goodSKID,
				NotBefore:    fc.Now().Add(-time.Hour * 2),
				NotAfter:     fc.Now().Add(-time.Hour),
			},
			expectedError: "NotBefore is backdated more than the maximum allowed period (2h0m0s>1h0m0s)",
		},
		{
			name: "validity is forward dated",
			issuer: &Issuer{
				active: true,
			},
			profile: &Profile{
				maxValidity: time.Hour * 2,
				maxBackdate: time.Hour,
			},
			request: &IssuanceRequest{
				PublicKey:    MarshalablePublicKey{&ecdsa.PublicKey{}},
				SubjectKeyId: goodSKID,
				NotBefore:    fc.Now().Add(time.Hour),
				NotAfter:     fc.Now().Add(time.Hour * 2),
			},
			expectedError: "NotBefore is in the future",
		},
		{
			name: "serial too short",
			issuer: &Issuer{
				active: true,
			},
			profile: &Profile{
				maxValidity: time.Hour * 2,
			},
			request: &IssuanceRequest{
				PublicKey:    MarshalablePublicKey{&ecdsa.PublicKey{}},
				SubjectKeyId: goodSKID,
				NotBefore:    fc.Now(),
				NotAfter:     fc.Now().Add(time.Hour),
				Serial:       []byte{0, 1, 2, 3, 4, 5, 6, 7},
			},
			expectedError: "serial must be between 9 and 19 bytes",
		},
		{
			name: "serial too long",
			issuer: &Issuer{
				active: true,
			},
			profile: &Profile{
				maxValidity: time.Hour * 2,
			},
			request: &IssuanceRequest{
				PublicKey:    MarshalablePublicKey{&ecdsa.PublicKey{}},
				SubjectKeyId: goodSKID,
				NotBefore:    fc.Now(),
				NotAfter:     fc.Now().Add(time.Hour),
				Serial:       []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
			},
			expectedError: "serial must be between 9 and 19 bytes",
		},
		{
			name: "good with poison",
			issuer: &Issuer{
				active: true,
			},
			profile: &Profile{
				maxValidity: time.Hour * 2,
			},
			request: &IssuanceRequest{
				PublicKey:       MarshalablePublicKey{&ecdsa.PublicKey{}},
				SubjectKeyId:    goodSKID,
				NotBefore:       fc.Now(),
				NotAfter:        fc.Now().Add(time.Hour),
				Serial:          []byte{1, 2, 3, 4, 5, 6, 7, 8, 9},
				IncludeCTPoison: true,
			},
		},
		{
			name: "good with scts",
			issuer: &Issuer{
				active: true,
			},
			profile: &Profile{
				maxValidity: time.Hour * 2,
			},
			request: &IssuanceRequest{
				PublicKey:    MarshalablePublicKey{&ecdsa.PublicKey{}},
				SubjectKeyId: goodSKID,
				NotBefore:    fc.Now(),
				NotAfter:     fc.Now().Add(time.Hour),
				Serial:       []byte{1, 2, 3, 4, 5, 6, 7, 8, 9},
				sctList:      []ct.SignedCertificateTimestamp{},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.issuer.requestValid(fc, tc.profile, tc.request)
			if err != nil {
				if tc.expectedError == "" {
					t.Errorf("failed with unexpected error: %s", err)
				} else if tc.expectedError != err.Error() {
					t.Errorf("failed with unexpected error, wanted: %q, got: %q", tc.expectedError, err.Error())
				}
				return
			} else if tc.expectedError != "" {
				t.Errorf("didn't fail, expected %q", tc.expectedError)
			}
		})
	}
}

func TestGenerateTemplate(t *testing.T) {
	issuer := &Issuer{
		issuerURL:  "http://issuer",
		crlURLBase: "http://crl/",
		sigAlg:     x509.SHA256WithRSA,
	}

	actual := issuer.generateTemplate()

	expected := &x509.Certificate{
		BasicConstraintsValid: true,
		SignatureAlgorithm:    x509.SHA256WithRSA,
		IssuingCertificateURL: []string{"http://issuer"},
		Policies:              []x509.OID{domainValidatedOID},
		// These fields are only included if specified in the profile.
		OCSPServer:            nil,
		CRLDistributionPoints: nil,
	}

	test.AssertDeepEquals(t, actual, expected)
}

func TestIssue(t *testing.T) {
	for _, tc := range []struct {
		name         string
		generateFunc func() (crypto.Signer, error)
		ku           x509.KeyUsage
	}{
		{
			name: "RSA",
			generateFunc: func() (crypto.Signer, error) {
				return rsa.GenerateKey(rand.Reader, 2048)
			},
			ku: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		},
		{
			name: "ECDSA",
			generateFunc: func() (crypto.Signer, error) {
				return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			},
			ku: x509.KeyUsageDigitalSignature,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fc := clock.NewFake()
			fc.Set(time.Now())
			signer, err := newIssuer(defaultIssuerConfig(), issuerCert, issuerSigner, fc)
			test.AssertNotError(t, err, "NewIssuer failed")
			pk, err := tc.generateFunc()
			test.AssertNotError(t, err, "failed to generate test key")
			lintCertBytes, issuanceToken, err := signer.Prepare(defaultProfile(), &IssuanceRequest{
				PublicKey:       MarshalablePublicKey{pk.Public()},
				SubjectKeyId:    goodSKID,
				Serial:          []byte{1, 2, 3, 4, 5, 6, 7, 8, 9},
				DNSNames:        []string{"example.com"},
				IPAddresses:     []net.IP{net.ParseIP("128.101.101.101"), net.ParseIP("3fff:aaa:a:c0ff:ee:a:bad:deed")},
				NotBefore:       fc.Now(),
				NotAfter:        fc.Now().Add(time.Hour - time.Second),
				IncludeCTPoison: true,
			})
			test.AssertNotError(t, err, "Prepare failed")
			_, err = x509.ParseCertificate(lintCertBytes)
			test.AssertNotError(t, err, "failed to parse certificate")
			certBytes, err := signer.Issue(issuanceToken)
			test.AssertNotError(t, err, "Issue failed")
			cert, err := x509.ParseCertificate(certBytes)
			test.AssertNotError(t, err, "failed to parse certificate")
			err = cert.CheckSignatureFrom(issuerCert.Certificate)
			test.AssertNotError(t, err, "signature validation failed")
			test.AssertDeepEquals(t, cert.DNSNames, []string{"example.com"})
			// net.ParseIP always returns a 16-byte address; IPv4 addresses are
			// returned in IPv4-mapped IPv6 form. But RFC 5280, Sec. 4.2.1.6
			// requires that IPv4 addresses be encoded as 4 bytes.
			//
			// The issuance pipeline calls x509.marshalSANs, which reduces IPv4
			// addresses back to 4 bytes. Adding .To4() both allows this test to
			// succeed, and covers this requirement.
			test.AssertDeepEquals(t, cert.IPAddresses, []net.IP{net.ParseIP("128.101.101.101").To4(), net.ParseIP("3fff:aaa:a:c0ff:ee:a:bad:deed")})
			test.AssertByteEquals(t, cert.SerialNumber.Bytes(), []byte{1, 2, 3, 4, 5, 6, 7, 8, 9})
			test.AssertDeepEquals(t, cert.PublicKey, pk.Public())
			test.AssertEquals(t, len(cert.Extensions), 10) // Constraints, KU, EKU, SKID, AKID, AIA, CRLDP, SAN, Policies, Poison
			test.AssertEquals(t, cert.KeyUsage, tc.ku)
			if len(cert.CRLDistributionPoints) != 1 || !strings.HasPrefix(cert.CRLDistributionPoints[0], "http://crl-url.example.org/") {
				t.Errorf("want CRLDistributionPoints=[http://crl-url.example.org/x.crl], got %v", cert.CRLDistributionPoints)
			}
		})
	}
}

func TestIssueDNSNamesOnly(t *testing.T) {
	fc := clock.NewFake()
	signer, err := newIssuer(defaultIssuerConfig(), issuerCert, issuerSigner, fc)
	if err != nil {
		t.Fatalf("newIssuer: %s", err)
	}
	pk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa.GenerateKey: %s", err)
	}
	_, issuanceToken, err := signer.Prepare(defaultProfile(), &IssuanceRequest{
		PublicKey:       MarshalablePublicKey{pk.Public()},
		SubjectKeyId:    goodSKID,
		Serial:          []byte{1, 2, 3, 4, 5, 6, 7, 8, 9},
		DNSNames:        []string{"example.com"},
		NotBefore:       fc.Now(),
		NotAfter:        fc.Now().Add(time.Hour - time.Second),
		IncludeCTPoison: true,
	})
	if err != nil {
		t.Fatalf("signer.Prepare: %s", err)
	}
	certBytes, err := signer.Issue(issuanceToken)
	if err != nil {
		t.Fatalf("signer.Issue: %s", err)
	}
	cert, err := x509.ParseCertificate(certBytes)
	if err != nil {
		t.Fatalf("x509.ParseCertificate: %s", err)
	}
	if !reflect.DeepEqual(cert.DNSNames, []string{"example.com"}) {
		t.Errorf("got DNSNames %s, wanted example.com", cert.DNSNames)
	}
	// BRs 7.1.2.7.12 requires iPAddress, if present, to contain an entry.
	if cert.IPAddresses != nil {
		t.Errorf("got IPAddresses %s, wanted nil", cert.IPAddresses)
	}
}

func TestIssueIPAddressesOnly(t *testing.T) {
	fc := clock.NewFake()
	signer, err := newIssuer(defaultIssuerConfig(), issuerCert, issuerSigner, fc)
	if err != nil {
		t.Fatalf("newIssuer: %s", err)
	}
	pk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa.GenerateKey: %s", err)
	}
	_, issuanceToken, err := signer.Prepare(defaultProfile(), &IssuanceRequest{
		PublicKey:       MarshalablePublicKey{pk.Public()},
		SubjectKeyId:    goodSKID,
		Serial:          []byte{1, 2, 3, 4, 5, 6, 7, 8, 9},
		IPAddresses:     []net.IP{net.ParseIP("128.101.101.101"), net.ParseIP("3fff:aaa:a:c0ff:ee:a:bad:deed")},
		NotBefore:       fc.Now(),
		NotAfter:        fc.Now().Add(time.Hour - time.Second),
		IncludeCTPoison: true,
	})
	if err != nil {
		t.Fatalf("signer.Prepare: %s", err)
	}
	certBytes, err := signer.Issue(issuanceToken)
	if err != nil {
		t.Fatalf("signer.Issue: %s", err)
	}
	cert, err := x509.ParseCertificate(certBytes)
	if err != nil {
		t.Fatalf("x509.ParseCertificate: %s", err)
	}
	// BRs 7.1.2.7.12 requires dNSName, if present, to contain an entry.
	if cert.DNSNames != nil {
		t.Errorf("got DNSNames %s, wanted nil", cert.DNSNames)
	}
	if !reflect.DeepEqual(cert.IPAddresses, []net.IP{net.ParseIP("128.101.101.101").To4(), net.ParseIP("3fff:aaa:a:c0ff:ee:a:bad:deed")}) {
		t.Errorf("got IPAddresses %s, wanted 128.101.101.101 (4-byte) & 3fff:aaa:a:c0ff:ee:a:bad:deed (16-byte)", cert.IPAddresses)
	}
}

func TestIssueWithCRLDP(t *testing.T) {
	fc := clock.NewFake()
	issuerConfig := defaultIssuerConfig()
	issuerConfig.CRLURLBase = "http://crls.example.net/"
	issuerConfig.CRLShards = 999
	signer, err := newIssuer(issuerConfig, issuerCert, issuerSigner, fc)
	if err != nil {
		t.Fatalf("newIssuer: %s", err)
	}
	pk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa.GenerateKey: %s", err)
	}
	profile := defaultProfile()
	profile.includeCRLDistributionPoints = true
	_, issuanceToken, err := signer.Prepare(profile, &IssuanceRequest{
		PublicKey:       MarshalablePublicKey{pk.Public()},
		SubjectKeyId:    goodSKID,
		Serial:          []byte{1, 2, 3, 4, 5, 6, 7, 8, 9},
		DNSNames:        []string{"example.com"},
		NotBefore:       fc.Now(),
		NotAfter:        fc.Now().Add(time.Hour - time.Second),
		IncludeCTPoison: true,
	})
	if err != nil {
		t.Fatalf("signer.Prepare: %s", err)
	}
	certBytes, err := signer.Issue(issuanceToken)
	if err != nil {
		t.Fatalf("signer.Issue: %s", err)
	}
	cert, err := x509.ParseCertificate(certBytes)
	if err != nil {
		t.Fatalf("x509.ParseCertificate: %s", err)
	}
	// Because CRL shard is calculated deterministically from serial, we know which shard will be chosen.
	expectedCRLDP := []string{"http://crls.example.net/919.crl"}
	if !reflect.DeepEqual(cert.CRLDistributionPoints, expectedCRLDP) {
		t.Errorf("CRLDP=%+v, want %+v", cert.CRLDistributionPoints, expectedCRLDP)
	}
}

func TestIssueCommonName(t *testing.T) {
	fc := clock.NewFake()
	fc.Set(time.Now())

	prof := defaultProfileConfig()
	prof.IgnoredLints = append(prof.IgnoredLints, "w_subject_common_name_included")
	cnProfile, err := NewProfile(prof)
	test.AssertNotError(t, err, "NewProfile failed")
	signer, err := newIssuer(defaultIssuerConfig(), issuerCert, issuerSigner, fc)
	test.AssertNotError(t, err, "NewIssuer failed")
	pk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	test.AssertNotError(t, err, "failed to generate test key")
	ir := &IssuanceRequest{
		PublicKey:       MarshalablePublicKey{pk.Public()},
		SubjectKeyId:    goodSKID,
		Serial:          []byte{1, 2, 3, 4, 5, 6, 7, 8, 9},
		DNSNames:        []string{"example.com", "www.example.com"},
		NotBefore:       fc.Now(),
		NotAfter:        fc.Now().Add(time.Hour - time.Second),
		IncludeCTPoison: true,
	}

	// In the default profile, the common name is allowed if requested.
	ir.CommonName = "example.com"
	_, issuanceToken, err := signer.Prepare(cnProfile, ir)
	test.AssertNotError(t, err, "Prepare failed")
	certBytes, err := signer.Issue(issuanceToken)
	test.AssertNotError(t, err, "Issue failed")
	cert, err := x509.ParseCertificate(certBytes)
	test.AssertNotError(t, err, "failed to parse certificate")
	test.AssertEquals(t, cert.Subject.CommonName, "example.com")

	// But not including the common name should be acceptable as well.
	ir.CommonName = ""
	_, issuanceToken, err = signer.Prepare(cnProfile, ir)
	test.AssertNotError(t, err, "Prepare failed")
	certBytes, err = signer.Issue(issuanceToken)
	test.AssertNotError(t, err, "Issue failed")
	cert, err = x509.ParseCertificate(certBytes)
	test.AssertNotError(t, err, "failed to parse certificate")
	test.AssertEquals(t, cert.Subject.CommonName, "")

	// And the common name should be omitted if the profile is so configured.
	ir.CommonName = "example.com"
	cnProfile.omitCommonName = true
	_, issuanceToken, err = signer.Prepare(cnProfile, ir)
	test.AssertNotError(t, err, "Prepare failed")
	certBytes, err = signer.Issue(issuanceToken)
	test.AssertNotError(t, err, "Issue failed")
	cert, err = x509.ParseCertificate(certBytes)
	test.AssertNotError(t, err, "failed to parse certificate")
	test.AssertEquals(t, cert.Subject.CommonName, "")
}

func TestIssueOmissions(t *testing.T) {
	fc := clock.NewFake()
	fc.Set(time.Now())

	pc := defaultProfileConfig()
	pc.OmitCommonName = true
	pc.OmitKeyEncipherment = true
	pc.OmitClientAuth = true
	pc.OmitSKID = true
	pc.IgnoredLints = []string{
		// Reduce the lint ignores to just the minimal (SCT-related) set.
		"w_ct_sct_policy_count_unsatisfied",
		"e_scts_from_same_operator",
		// Ignore the warning about *not* including the SubjectKeyIdentifier extension:
		// zlint has both lints (one enforcing RFC5280, the other the BRs).
		"w_ext_subject_key_identifier_missing_sub_cert",
	}
	prof, err := NewProfile(pc)
	test.AssertNotError(t, err, "building test profile")

	signer, err := newIssuer(defaultIssuerConfig(), issuerCert, issuerSigner, fc)
	test.AssertNotError(t, err, "NewIssuer failed")

	pk, err := rsa.GenerateKey(rand.Reader, 2048)
	test.AssertNotError(t, err, "failed to generate test key")
	_, issuanceToken, err := signer.Prepare(prof, &IssuanceRequest{
		PublicKey:       MarshalablePublicKey{pk.Public()},
		SubjectKeyId:    goodSKID,
		Serial:          []byte{1, 2, 3, 4, 5, 6, 7, 8, 9},
		DNSNames:        []string{"example.com"},
		CommonName:      "example.com",
		IncludeCTPoison: true,
		NotBefore:       fc.Now(),
		NotAfter:        fc.Now().Add(time.Hour - time.Second),
	})
	test.AssertNotError(t, err, "Prepare failed")
	certBytes, err := signer.Issue(issuanceToken)
	test.AssertNotError(t, err, "Issue failed")
	cert, err := x509.ParseCertificate(certBytes)
	test.AssertNotError(t, err, "failed to parse certificate")

	test.AssertEquals(t, cert.Subject.CommonName, "")
	test.AssertEquals(t, cert.KeyUsage, x509.KeyUsageDigitalSignature)
	test.AssertDeepEquals(t, cert.ExtKeyUsage, []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth})
	test.AssertEquals(t, len(cert.SubjectKeyId), 0)
}

func TestIssueCTPoison(t *testing.T) {
	fc := clock.NewFake()
	fc.Set(time.Now())
	signer, err := newIssuer(defaultIssuerConfig(), issuerCert, issuerSigner, fc)
	test.AssertNotError(t, err, "NewIssuer failed")
	pk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	test.AssertNotError(t, err, "failed to generate test key")
	_, issuanceToken, err := signer.Prepare(defaultProfile(), &IssuanceRequest{
		PublicKey:       MarshalablePublicKey{pk.Public()},
		SubjectKeyId:    goodSKID,
		Serial:          []byte{1, 2, 3, 4, 5, 6, 7, 8, 9},
		DNSNames:        []string{"example.com"},
		IncludeCTPoison: true,
		NotBefore:       fc.Now(),
		NotAfter:        fc.Now().Add(time.Hour - time.Second),
	})
	test.AssertNotError(t, err, "Prepare failed")
	certBytes, err := signer.Issue(issuanceToken)
	test.AssertNotError(t, err, "Issue failed")
	cert, err := x509.ParseCertificate(certBytes)
	test.AssertNotError(t, err, "failed to parse certificate")
	err = cert.CheckSignatureFrom(issuerCert.Certificate)
	test.AssertNotError(t, err, "signature validation failed")
	test.AssertByteEquals(t, cert.SerialNumber.Bytes(), []byte{1, 2, 3, 4, 5, 6, 7, 8, 9})
	test.AssertDeepEquals(t, cert.PublicKey, pk.Public())
	test.AssertEquals(t, len(cert.Extensions), 10) // Constraints, KU, EKU, SKID, AKID, AIA, CRLDP, SAN, Policies, Poison
	test.AssertDeepEquals(t, cert.Extensions[9], ctPoisonExt)
}

func mustDecodeB64(b string) []byte {
	out, err := base64.StdEncoding.DecodeString(b)
	if err != nil {
		panic(err)
	}
	return out
}

func TestIssueSCTList(t *testing.T) {
	fc := clock.NewFake()
	fc.Set(time.Now())

	err := loglist.InitLintList("../test/ct-test-srv/log_list.json")
	test.AssertNotError(t, err, "failed to load log list")

	pc := defaultProfileConfig()
	pc.IgnoredLints = []string{
		// Only ignore the SKID lint, i.e., don't ignore the "missing SCT" lints.
		"w_ext_subject_key_identifier_not_recommended_subscriber",
	}
	enforceSCTsProfile, err := NewProfile(pc)
	test.AssertNotError(t, err, "NewProfile failed")
	signer, err := newIssuer(defaultIssuerConfig(), issuerCert, issuerSigner, fc)
	test.AssertNotError(t, err, "NewIssuer failed")
	pk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	test.AssertNotError(t, err, "failed to generate test key")
	_, issuanceToken, err := signer.Prepare(enforceSCTsProfile, &IssuanceRequest{
		PublicKey:       MarshalablePublicKey{pk.Public()},
		SubjectKeyId:    goodSKID,
		Serial:          []byte{1, 2, 3, 4, 5, 6, 7, 8, 9},
		DNSNames:        []string{"example.com"},
		NotBefore:       fc.Now(),
		NotAfter:        fc.Now().Add(time.Hour - time.Second),
		IncludeCTPoison: true,
	})
	test.AssertNotError(t, err, "Prepare failed")
	precertBytes, err := signer.Issue(issuanceToken)
	test.AssertNotError(t, err, "Issue failed")
	precert, err := x509.ParseCertificate(precertBytes)
	test.AssertNotError(t, err, "failed to parse certificate")

	sctList := []ct.SignedCertificateTimestamp{
		{
			SCTVersion: ct.V1,
			LogID:      ct.LogID{KeyID: *(*[32]byte)(mustDecodeB64("OJiMlNA1mMOTLd/pI7q68npCDrlsQeFaqAwasPwEvQM="))},
		},
		{
			SCTVersion: ct.V1,
			LogID:      ct.LogID{KeyID: *(*[32]byte)(mustDecodeB64("UtToynGEyMkkXDMQei8Ll54oMwWHI0IieDEKs12/Td4="))},
		},
	}

	request2, err := RequestFromPrecert(precert, sctList)
	test.AssertNotError(t, err, "generating request from precert")

	_, issuanceToken2, err := signer.Prepare(enforceSCTsProfile, request2)
	test.AssertNotError(t, err, "preparing final cert issuance")

	finalCertBytes, err := signer.Issue(issuanceToken2)
	test.AssertNotError(t, err, "Issue failed")

	finalCert, err := x509.ParseCertificate(finalCertBytes)
	test.AssertNotError(t, err, "failed to parse certificate")

	err = finalCert.CheckSignatureFrom(issuerCert.Certificate)
	test.AssertNotError(t, err, "signature validation failed")
	test.AssertByteEquals(t, finalCert.SerialNumber.Bytes(), []byte{1, 2, 3, 4, 5, 6, 7, 8, 9})
	test.AssertDeepEquals(t, finalCert.PublicKey, pk.Public())
	test.AssertEquals(t, len(finalCert.Extensions), 10) // Constraints, KU, EKU, SKID, AKID, AIA, CRLDP, SAN, Policies, Poison
	test.AssertDeepEquals(t, finalCert.Extensions[9], pkix.Extension{
		Id: sctListOID,
		Value: []byte{
			4, 100, 0, 98, 0, 47, 0, 56, 152, 140, 148, 208, 53, 152, 195, 147, 45,
			223, 233, 35, 186, 186, 242, 122, 66, 14, 185, 108, 65, 225, 90, 168, 12,
			26, 176, 252, 4, 189, 3, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 47,
			0, 82, 212, 232, 202, 113, 132, 200, 201, 36, 92, 51, 16, 122, 47, 11,
			151, 158, 40, 51, 5, 135, 35, 66, 34, 120, 49, 10, 179, 93, 191, 77, 222,
			0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		},
	})
}

func TestIssueBadLint(t *testing.T) {
	fc := clock.NewFake()
	fc.Set(time.Now())

	pc := defaultProfileConfig()
	pc.IgnoredLints = []string{}
	noSkipLintsProfile, err := NewProfile(pc)
	test.AssertNotError(t, err, "NewProfile failed")
	signer, err := newIssuer(defaultIssuerConfig(), issuerCert, issuerSigner, fc)
	test.AssertNotError(t, err, "NewIssuer failed")
	pk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	test.AssertNotError(t, err, "failed to generate test key")
	_, _, err = signer.Prepare(noSkipLintsProfile, &IssuanceRequest{
		PublicKey:       MarshalablePublicKey{pk.Public()},
		SubjectKeyId:    goodSKID,
		Serial:          []byte{1, 2, 3, 4, 5, 6, 7, 8, 9},
		DNSNames:        []string{"example-com"},
		NotBefore:       fc.Now(),
		NotAfter:        fc.Now().Add(time.Hour - time.Second),
		IncludeCTPoison: true,
	})
	test.AssertError(t, err, "Prepare didn't fail")
	test.AssertErrorIs(t, err, linter.ErrLinting)
	test.AssertContains(t, err.Error(), "tbsCertificate linting failed: failed lint(s)")
}

func TestIssuanceToken(t *testing.T) {
	fc := clock.NewFake()
	fc.Set(time.Now())

	signer, err := newIssuer(defaultIssuerConfig(), issuerCert, issuerSigner, fc)
	test.AssertNotError(t, err, "NewIssuer failed")

	_, err = signer.Issue(&issuanceToken{})
	test.AssertError(t, err, "expected issuance with a zero token to fail")

	_, err = signer.Issue(nil)
	test.AssertError(t, err, "expected issuance with a nil token to fail")

	pk, err := rsa.GenerateKey(rand.Reader, 2048)
	test.AssertNotError(t, err, "failed to generate test key")
	_, issuanceToken, err := signer.Prepare(defaultProfile(), &IssuanceRequest{
		PublicKey:       MarshalablePublicKey{pk.Public()},
		SubjectKeyId:    goodSKID,
		Serial:          []byte{1, 2, 3, 4, 5, 6, 7, 8, 9},
		DNSNames:        []string{"example.com"},
		NotBefore:       fc.Now(),
		NotAfter:        fc.Now().Add(time.Hour - time.Second),
		IncludeCTPoison: true,
	})
	test.AssertNotError(t, err, "expected Prepare to succeed")
	_, err = signer.Issue(issuanceToken)
	test.AssertNotError(t, err, "expected first issuance to succeed")

	_, err = signer.Issue(issuanceToken)
	test.AssertError(t, err, "expected second issuance with the same issuance token to fail")
	test.AssertContains(t, err.Error(), "issuance token already redeemed")

	_, issuanceToken, err = signer.Prepare(defaultProfile(), &IssuanceRequest{
		PublicKey:       MarshalablePublicKey{pk.Public()},
		SubjectKeyId:    goodSKID,
		Serial:          []byte{1, 2, 3, 4, 5, 6, 7, 8, 9},
		DNSNames:        []string{"example.com"},
		NotBefore:       fc.Now(),
		NotAfter:        fc.Now().Add(time.Hour - time.Second),
		IncludeCTPoison: true,
	})
	test.AssertNotError(t, err, "expected Prepare to succeed")

	signer2, err := newIssuer(defaultIssuerConfig(), issuerCert, issuerSigner, fc)
	test.AssertNotError(t, err, "NewIssuer failed")

	_, err = signer2.Issue(issuanceToken)
	test.AssertError(t, err, "expected redeeming an issuance token with the wrong issuer to fail")
	test.AssertContains(t, err.Error(), "wrong issuer")
}

func TestInvalidProfile(t *testing.T) {
	fc := clock.NewFake()
	fc.Set(time.Now())

	err := loglist.InitLintList("../test/ct-test-srv/log_list.json")
	test.AssertNotError(t, err, "failed to load log list")

	signer, err := newIssuer(defaultIssuerConfig(), issuerCert, issuerSigner, fc)
	test.AssertNotError(t, err, "NewIssuer failed")
	pk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	test.AssertNotError(t, err, "failed to generate test key")
	_, _, err = signer.Prepare(defaultProfile(), &IssuanceRequest{
		PublicKey:       MarshalablePublicKey{pk.Public()},
		SubjectKeyId:    goodSKID,
		Serial:          []byte{1, 2, 3, 4, 5, 6, 7, 8, 9},
		DNSNames:        []string{"example.com"},
		NotBefore:       fc.Now(),
		NotAfter:        fc.Now().Add(time.Hour - time.Second),
		IncludeCTPoison: true,
		precertDER:      []byte{6, 6, 6},
	})
	test.AssertError(t, err, "Invalid IssuanceRequest")

	_, _, err = signer.Prepare(defaultProfile(), &IssuanceRequest{
		PublicKey:    MarshalablePublicKey{pk.Public()},
		SubjectKeyId: goodSKID,
		Serial:       []byte{1, 2, 3, 4, 5, 6, 7, 8, 9},
		DNSNames:     []string{"example.com"},
		NotBefore:    fc.Now(),
		NotAfter:     fc.Now().Add(time.Hour - time.Second),
		sctList: []ct.SignedCertificateTimestamp{
			{
				SCTVersion: ct.V1,
				LogID:      ct.LogID{KeyID: *(*[32]byte)(mustDecodeB64("OJiMlNA1mMOTLd/pI7q68npCDrlsQeFaqAwasPwEvQM="))},
			},
		},
		precertDER: []byte{},
	})
	test.AssertError(t, err, "Invalid IssuanceRequest")
}

// Generate a precert from one profile and a final cert from another, and verify
// that the final cert errors out when linted because the lint cert doesn't
// corresponding with the precert.
func TestMismatchedProfiles(t *testing.T) {
	fc := clock.NewFake()
	fc.Set(time.Now())
	err := loglist.InitLintList("../test/ct-test-srv/log_list.json")
	test.AssertNotError(t, err, "failed to load log list")

	issuer1, err := newIssuer(defaultIssuerConfig(), issuerCert, issuerSigner, fc)
	test.AssertNotError(t, err, "NewIssuer failed")

	pc := defaultProfileConfig()
	pc.IgnoredLints = append(pc.IgnoredLints, "w_subject_common_name_included")
	cnProfile, err := NewProfile(pc)
	test.AssertNotError(t, err, "NewProfile failed")

	pk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	test.AssertNotError(t, err, "failed to generate test key")
	_, issuanceToken, err := issuer1.Prepare(cnProfile, &IssuanceRequest{
		PublicKey:       MarshalablePublicKey{pk.Public()},
		SubjectKeyId:    goodSKID,
		Serial:          []byte{1, 2, 3, 4, 5, 6, 7, 8, 9},
		CommonName:      "example.com",
		DNSNames:        []string{"example.com"},
		NotBefore:       fc.Now(),
		NotAfter:        fc.Now().Add(time.Hour - time.Second),
		IncludeCTPoison: true,
	})
	test.AssertNotError(t, err, "making IssuanceRequest")

	precertDER, err := issuer1.Issue(issuanceToken)
	test.AssertNotError(t, err, "signing precert")

	// Create a new profile that differs slightly (no common name)
	pc = defaultProfileConfig()
	pc.OmitCommonName = false
	test.AssertNotError(t, err, "building test lint registry")
	noCNProfile, err := NewProfile(pc)
	test.AssertNotError(t, err, "NewProfile failed")

	issuer2, err := newIssuer(defaultIssuerConfig(), issuerCert, issuerSigner, fc)
	test.AssertNotError(t, err, "NewIssuer failed")

	sctList := []ct.SignedCertificateTimestamp{
		{
			SCTVersion: ct.V1,
			LogID:      ct.LogID{KeyID: *(*[32]byte)(mustDecodeB64("OJiMlNA1mMOTLd/pI7q68npCDrlsQeFaqAwasPwEvQM="))},
		},
		{
			SCTVersion: ct.V1,
			LogID:      ct.LogID{KeyID: *(*[32]byte)(mustDecodeB64("UtToynGEyMkkXDMQei8Ll54oMwWHI0IieDEKs12/Td4="))},
		},
	}

	precert, err := x509.ParseCertificate(precertDER)
	test.AssertNotError(t, err, "parsing precert")

	request2, err := RequestFromPrecert(precert, sctList)
	test.AssertNotError(t, err, "RequestFromPrecert")
	request2.CommonName = ""

	_, _, err = issuer2.Prepare(noCNProfile, request2)
	test.AssertError(t, err, "preparing final cert issuance")
	test.AssertContains(t, err.Error(), "precert does not correspond to linted final cert")
}

func TestNewProfile(t *testing.T) {
	for _, tc := range []struct {
		name    string
		config  ProfileConfig
		wantErr string
	}{
		{
			name: "happy path",
			config: ProfileConfig{
				MaxValidityBackdate:          config.Duration{Duration: 1 * time.Hour},
				MaxValidityPeriod:            config.Duration{Duration: 90 * 24 * time.Hour},
				IncludeCRLDistributionPoints: true,
			},
		},
		{
			name: "large backdate",
			config: ProfileConfig{
				MaxValidityBackdate: config.Duration{Duration: 24 * time.Hour},
				MaxValidityPeriod:   config.Duration{Duration: 90 * 24 * time.Hour},
			},
			wantErr: "backdate \"24h0m0s\" is too large",
		},
		{
			name: "large validity",
			config: ProfileConfig{
				MaxValidityBackdate: config.Duration{Duration: 1 * time.Hour},
				MaxValidityPeriod:   config.Duration{Duration: 397 * 24 * time.Hour},
			},
			wantErr: "validity period \"9528h0m0s\" is too large",
		},
		{
			name: "no revocation info",
			config: ProfileConfig{
				MaxValidityBackdate:          config.Duration{Duration: 1 * time.Hour},
				MaxValidityPeriod:            config.Duration{Duration: 90 * 24 * time.Hour},
				IncludeCRLDistributionPoints: false,
			},
			wantErr: "revocation mechanism must be included",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotProfile, gotErr := NewProfile(&tc.config)
			if tc.wantErr != "" {
				if gotErr == nil {
					t.Errorf("NewProfile(%#v) = %#v, but want err %q", tc.config, gotProfile, tc.wantErr)
				}
				if !strings.Contains(gotErr.Error(), tc.wantErr) {
					t.Errorf("NewProfile(%#v) = %q, but want %q", tc.config, gotErr, tc.wantErr)
				}
			} else {
				if gotErr != nil {
					t.Errorf("NewProfile(%#v) = %q, but want no error", tc.config, gotErr)
				}
			}
		})
	}
}
