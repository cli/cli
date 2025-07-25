package main

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"io"
	"math/big"
	"testing"
	"time"

	"github.com/letsencrypt/boulder/test"
)

func TestGenerateCRLTimeBounds(t *testing.T) {
	_, err := generateCRL(nil, nil, time.Now().Add(time.Hour), time.Now(), 1, nil)
	test.AssertError(t, err, "generateCRL did not fail")
	test.AssertEquals(t, err.Error(), "thisUpdate must be before nextUpdate")

	_, err = generateCRL(nil, &x509.Certificate{
		NotBefore: time.Now().Add(time.Hour),
		NotAfter:  time.Now(),
	}, time.Now(), time.Now(), 1, nil)
	test.AssertError(t, err, "generateCRL did not fail")
	test.AssertEquals(t, err.Error(), "thisUpdate is before issuing certificate's notBefore")

	_, err = generateCRL(nil, &x509.Certificate{
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(time.Hour * 2),
	}, time.Now().Add(time.Hour), time.Now().Add(time.Hour*3), 1, nil)
	test.AssertError(t, err, "generateCRL did not fail")
	test.AssertEquals(t, err.Error(), "nextUpdate is after issuing certificate's notAfter")

	_, err = generateCRL(nil, &x509.Certificate{
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(time.Hour * 24 * 370),
	}, time.Now(), time.Now().Add(time.Hour*24*366), 1, nil)
	test.AssertError(t, err, "generateCRL did not fail")
	test.AssertEquals(t, err.Error(), "nextUpdate must be less than 12 months after thisUpdate")
}

// wrappedSigner wraps a crypto.Signer. In order to use a crypto.Signer in tests
// we need to wrap it as we pass a purposefully broken io.Reader to Sign in order
// to verify that go isn't using it as a source of randomness (we expect this
// randomness to come from the HSM). If we directly call Sign on the crypto.Signer
// it would fail, so we wrap it so that we can use a shim rand.Reader in the Sign
// call.
type wrappedSigner struct{ k crypto.Signer }

func (p wrappedSigner) Sign(_ io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	return p.k.Sign(rand.Reader, digest, opts)
}

func (p wrappedSigner) Public() crypto.PublicKey {
	return p.k.Public()
}

func TestGenerateCRLLints(t *testing.T) {
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	test.AssertNotError(t, err, "failed to generate test key")

	cert := &x509.Certificate{
		Subject:      pkix.Name{CommonName: "asd"},
		SerialNumber: big.NewInt(7),
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		IsCA:         true,
		KeyUsage:     x509.KeyUsageCRLSign,
		SubjectKeyId: []byte{1, 2, 3},
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, cert, cert, k.Public(), k)
	test.AssertNotError(t, err, "failed to generate test cert")
	cert, err = x509.ParseCertificate(certBytes)
	test.AssertNotError(t, err, "failed to parse test cert")

	// This CRL should fail the following lint:
	// - e_crl_acceptable_reason_codes (because 6 is forbidden)
	_, err = generateCRL(&wrappedSigner{k}, cert, time.Now().Add(time.Hour), time.Now().Add(100*24*time.Hour), 1, []x509.RevocationListEntry{
		{
			SerialNumber:   big.NewInt(12345),
			RevocationTime: time.Now().Add(time.Hour),
			ReasonCode:     6,
		},
	})
	test.AssertError(t, err, "generateCRL did not fail")
	test.AssertContains(t, err.Error(), "e_crl_acceptable_reason_codes")
}

func TestGenerateCRL(t *testing.T) {
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	test.AssertNotError(t, err, "failed to generate test key")

	template := &x509.Certificate{
		Subject:               pkix.Name{CommonName: "asd"},
		SerialNumber:          big.NewInt(7),
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCRLSign,
		SubjectKeyId:          []byte{1, 2, 3},
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, template, template, k.Public(), k)
	test.AssertNotError(t, err, "failed to generate test cert")
	cert, err := x509.ParseCertificate(certBytes)
	test.AssertNotError(t, err, "failed to parse test cert")

	crlPEM, err := generateCRL(&wrappedSigner{k}, cert, time.Now().Add(time.Hour), time.Now().Add(time.Hour*2), 1, nil)
	test.AssertNotError(t, err, "generateCRL failed with valid profile")

	pemBlock, _ := pem.Decode(crlPEM)
	crlDER := pemBlock.Bytes

	// use crypto/x509 to check signature is valid and list is empty
	goCRL, err := x509.ParseRevocationList(crlDER)
	test.AssertNotError(t, err, "failed to parse CRL")
	err = goCRL.CheckSignatureFrom(cert)
	test.AssertNotError(t, err, "CRL signature check failed")
	test.AssertEquals(t, len(goCRL.RevokedCertificateEntries), 0)

	// fully parse the CRL to check that the version is correct, and that
	// it contains the CRL number extension containing the number we expect
	var crl asn1CRL
	_, err = asn1.Unmarshal(crlDER, &crl)
	test.AssertNotError(t, err, "failed to parse CRL")
	test.AssertEquals(t, crl.TBS.Version, 1)         // x509v2 == 1
	test.AssertEquals(t, len(crl.TBS.Extensions), 3) // AKID, CRL number, IssuingDistributionPoint
	test.Assert(t, crl.TBS.Extensions[1].Id.Equal(asn1.ObjectIdentifier{2, 5, 29, 20}), "unexpected OID in extension")
	test.Assert(t, crl.TBS.Extensions[2].Id.Equal(asn1.ObjectIdentifier{2, 5, 29, 28}), "unexpected OID in extension")
	var number int
	_, err = asn1.Unmarshal(crl.TBS.Extensions[1].Value, &number)
	test.AssertNotError(t, err, "failed to parse CRL number extension")
	test.AssertEquals(t, number, 1)
}

type asn1CRL struct {
	TBS struct {
		Version int `asn1:"optional"`
		SigAlg  pkix.AlgorithmIdentifier
		Issuer  struct {
			Raw asn1.RawContent
		}
		ThisUpdate          time.Time
		NextUpdate          time.Time `asn1:"optional"`
		RevokedCertificates []struct {
			Serial     *big.Int
			RevokedAt  time.Time
			Extensions []pkix.Extension `asn1:"optional"`
		} `asn1:"optional"`
		Extensions []pkix.Extension `asn1:"optional,explicit,tag:0"`
	}
	SigAlg pkix.AlgorithmIdentifier
	Sig    asn1.BitString
}
