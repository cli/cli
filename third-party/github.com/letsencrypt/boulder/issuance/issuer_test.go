package issuance

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jmhodges/clock"

	"github.com/letsencrypt/boulder/cmd"
	"github.com/letsencrypt/boulder/config"
	"github.com/letsencrypt/boulder/core"
	"github.com/letsencrypt/boulder/test"
)

func defaultProfileConfig() *ProfileConfig {
	return &ProfileConfig{
		AllowMustStaple:              true,
		IncludeCRLDistributionPoints: true,
		MaxValidityPeriod:            config.Duration{Duration: time.Hour},
		MaxValidityBackdate:          config.Duration{Duration: time.Hour},
		IgnoredLints: []string{
			// Ignore the two SCT lints because these tests don't get SCTs.
			"w_ct_sct_policy_count_unsatisfied",
			"e_scts_from_same_operator",
			// Ignore the warning about including the SubjectKeyIdentifier extension:
			// we include it on purpose, but plan to remove it soon.
			"w_ext_subject_key_identifier_not_recommended_subscriber",
		},
	}
}

func defaultIssuerConfig() IssuerConfig {
	return IssuerConfig{
		Active:     true,
		IssuerURL:  "http://issuer-url.example.org",
		CRLURLBase: "http://crl-url.example.org/",
		CRLShards:  10,
	}
}

var issuerCert *Certificate
var issuerSigner *ecdsa.PrivateKey

func TestMain(m *testing.M) {
	tk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	cmd.FailOnError(err, "failed to generate test key")
	issuerSigner = tk
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(123),
		BasicConstraintsValid: true,
		IsCA:                  true,
		Subject: pkix.Name{
			CommonName: "big ca",
		},
		KeyUsage: x509.KeyUsageCRLSign | x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	issuer, err := x509.CreateCertificate(rand.Reader, template, template, tk.Public(), tk)
	cmd.FailOnError(err, "failed to generate test issuer")
	cert, err := x509.ParseCertificate(issuer)
	cmd.FailOnError(err, "failed to parse test issuer")
	issuerCert = &Certificate{Certificate: cert}
	os.Exit(m.Run())
}

func TestLoadCertificate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		path    string
		wantErr string
	}{
		{"invalid cert file", "../test/hierarchy/int-e1.crl.pem", "loading issuer certificate"},
		{"non-CA cert file", "../test/hierarchy/ee-e1.cert.pem", "not a CA certificate"},
		{"happy path", "../test/hierarchy/int-e1.cert.pem", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := LoadCertificate(tc.path)
			if err != nil {
				if tc.wantErr != "" {
					test.AssertContains(t, err.Error(), tc.wantErr)
				} else {
					t.Errorf("expected no error but got %v", err)
				}
			} else {
				if tc.wantErr != "" {
					t.Errorf("expected error %q but got none", tc.wantErr)
				}
			}
		})
	}
}

func TestLoadSigner(t *testing.T) {
	t.Parallel()

	// We're using this for its pubkey. This definitely doesn't match the private
	// key loaded in any of the tests below, but that's okay because it still gets
	// us through all the logic in loadSigner.
	fakeKey, err := ecdsa.GenerateKey(elliptic.P224(), rand.Reader)
	test.AssertNotError(t, err, "generating test key")

	tests := []struct {
		name    string
		loc     IssuerLoc
		wantErr string
	}{
		{"empty IssuerLoc", IssuerLoc{}, "must supply"},
		{"invalid key file", IssuerLoc{File: "../test/hierarchy/int-e1.crl.pem"}, "unable to parse"},
		{"ECDSA key file", IssuerLoc{File: "../test/hierarchy/int-e1.key.pem"}, ""},
		{"RSA key file", IssuerLoc{File: "../test/hierarchy/int-r3.key.pem"}, ""},
		{"invalid config file", IssuerLoc{ConfigFile: "../test/hostname-policy.yaml"}, "invalid character"},
		// Note that we don't have a test for "valid config file" because it would
		// always fail -- in CI, the softhsm hasn't been initialized, so there's no
		// key to look up; locally even if the softhsm has been initialized, the
		// keys in it don't match the fakeKey we generated above.
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := loadSigner(tc.loc, fakeKey.Public())
			if err != nil {
				if tc.wantErr != "" {
					test.AssertContains(t, err.Error(), tc.wantErr)
				} else {
					t.Errorf("expected no error but got %v", err)
				}
			} else {
				if tc.wantErr != "" {
					t.Errorf("expected error %q but got none", tc.wantErr)
				}
			}
		})
	}
}

func TestLoadIssuer(t *testing.T) {
	_, err := newIssuer(
		defaultIssuerConfig(),
		issuerCert,
		issuerSigner,
		clock.NewFake(),
	)
	test.AssertNotError(t, err, "newIssuer failed")
}

func TestNewIssuerUnsupportedKeyType(t *testing.T) {
	_, err := newIssuer(
		defaultIssuerConfig(),
		&Certificate{
			Certificate: &x509.Certificate{
				PublicKey: &ed25519.PublicKey{},
			},
		},
		&ed25519.PrivateKey{},
		clock.NewFake(),
	)
	test.AssertError(t, err, "newIssuer didn't fail")
	test.AssertEquals(t, err.Error(), "unsupported issuer key type")
}

func TestNewIssuerKeyUsage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		ku      x509.KeyUsage
		wantErr string
	}{
		{"missing certSign", x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature, "does not have keyUsage certSign"},
		{"missing crlSign", x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature, "does not have keyUsage crlSign"},
		{"missing digitalSignature", x509.KeyUsageCertSign | x509.KeyUsageCRLSign, "does not have keyUsage digitalSignature"},
		{"all three", x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := newIssuer(
				defaultIssuerConfig(),
				&Certificate{
					Certificate: &x509.Certificate{
						SerialNumber: big.NewInt(123),
						PublicKey: &ecdsa.PublicKey{
							Curve: elliptic.P256(),
						},
						KeyUsage: tc.ku,
					},
				},
				issuerSigner,
				clock.NewFake(),
			)
			if err != nil {
				if tc.wantErr != "" {
					test.AssertContains(t, err.Error(), tc.wantErr)
				} else {
					t.Errorf("expected no error but got %v", err)
				}
			} else {
				if tc.wantErr != "" {
					t.Errorf("expected error %q but got none", tc.wantErr)
				}
			}
		})
	}
}

func TestLoadChain_Valid(t *testing.T) {
	chain, err := LoadChain([]string{
		"../test/hierarchy/int-e1.cert.pem",
		"../test/hierarchy/root-x2.cert.pem",
	})
	test.AssertNotError(t, err, "Should load valid chain")

	expectedIssuer, err := core.LoadCert("../test/hierarchy/int-e1.cert.pem")
	test.AssertNotError(t, err, "Failed to load test issuer")

	chainIssuer := chain[0]
	test.AssertNotNil(t, chainIssuer, "Failed to decode chain PEM")

	test.AssertByteEquals(t, chainIssuer.Raw, expectedIssuer.Raw)
}

func TestLoadChain_TooShort(t *testing.T) {
	_, err := LoadChain([]string{"/path/to/one/cert.pem"})
	test.AssertError(t, err, "Should reject too-short chain")
}

func TestLoadChain_Unloadable(t *testing.T) {
	_, err := LoadChain([]string{
		"does-not-exist.pem",
		"../test/hierarchy/root-x2.cert.pem",
	})
	test.AssertError(t, err, "Should reject unloadable chain")

	_, err = LoadChain([]string{
		"../test/hierarchy/int-e1.cert.pem",
		"does-not-exist.pem",
	})
	test.AssertError(t, err, "Should reject unloadable chain")

	invalidPEMFile, _ := os.CreateTemp("", "invalid.pem")
	err = os.WriteFile(invalidPEMFile.Name(), []byte(""), 0640)
	test.AssertNotError(t, err, "Error writing invalid PEM tmp file")
	_, err = LoadChain([]string{
		invalidPEMFile.Name(),
		"../test/hierarchy/root-x2.cert.pem",
	})
	test.AssertError(t, err, "Should reject unloadable chain")
}

func TestLoadChain_InvalidSig(t *testing.T) {
	_, err := LoadChain([]string{
		"../test/hierarchy/int-e1.cert.pem",
		"../test/hierarchy/root-x1.cert.pem",
	})
	test.AssertError(t, err, "Should reject invalid signature")
	test.Assert(t, strings.Contains(err.Error(), "root-x1.cert.pem"),
		fmt.Sprintf("Expected error to mention filename, got: %s", err))
	test.Assert(t, strings.Contains(err.Error(), "signature from \"CN=(TEST) Ineffable Ice X1"),
		fmt.Sprintf("Expected error to mention subject, got: %s", err))
}
