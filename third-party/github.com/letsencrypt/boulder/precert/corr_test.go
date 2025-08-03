package precert

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"strings"
	"testing"
	"time"
)

func TestCorrespondIncorrectArgumentOrder(t *testing.T) {
	pre, final, err := readPair("testdata/good/precert.pem", "testdata/good/final.pem")
	if err != nil {
		t.Fatal(err)
	}

	// The final cert is in the precert position and vice versa.
	err = Correspond(final, pre)
	if err == nil {
		t.Errorf("expected failure when final and precertificates were in wrong order, got success")
	}
}

func TestCorrespondGood(t *testing.T) {
	pre, final, err := readPair("testdata/good/precert.pem", "testdata/good/final.pem")
	if err != nil {
		t.Fatal(err)
	}

	err = Correspond(pre, final)
	if err != nil {
		t.Errorf("expected testdata/good/ certs to correspond, got %s", err)
	}
}

func TestCorrespondBad(t *testing.T) {
	pre, final, err := readPair("testdata/bad/precert.pem", "testdata/bad/final.pem")
	if err != nil {
		t.Fatal(err)
	}

	err = Correspond(pre, final)
	if err == nil {
		t.Errorf("expected testdata/bad/ certs to not correspond, got nil error")
	}
	expected := "precert extension 7 (0603551d20040c300a3008060667810c010201) not equal to final cert extension 7 (0603551d20044530433008060667810c0102013037060b2b0601040182df130101013028302606082b06010505070201161a687474703a2f2f6370732e6c657473656e63727970742e6f7267)"
	if !strings.Contains(err.Error(), expected) {
		t.Errorf("expected error to contain %q, got %q", expected, err.Error())
	}
}

func TestCorrespondCompleteMismatch(t *testing.T) {
	pre, final, err := readPair("testdata/good/precert.pem", "testdata/bad/final.pem")
	if err != nil {
		t.Fatal(err)
	}

	err = Correspond(pre, final)
	if err == nil {
		t.Errorf("expected testdata/good and testdata/bad/ certs to not correspond, got nil error")
	}
	expected := "checking for identical field 1: elements differ: 021203d91c3d22b404f20df3c1631c22e1754b8d != 021203e2267b786b7e338317ddd62e764fcb3c71"
	if !strings.Contains(err.Error(), expected) {
		t.Errorf("expected error to contain %q, got %q", expected, err.Error())
	}
}

func readPair(a, b string) ([]byte, []byte, error) {
	aDER, err := derFromPEMFile(a)
	if err != nil {
		return nil, nil, err
	}
	bDER, err := derFromPEMFile(b)
	if err != nil {
		return nil, nil, err
	}
	return aDER, bDER, nil
}

// derFromPEMFile reads a PEM file and returns the DER-encoded bytes.
func derFromPEMFile(filename string) ([]byte, error) {
	precertPEM, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", filename, err)
	}

	precertPEMBlock, _ := pem.Decode(precertPEM)
	if precertPEMBlock == nil {
		return nil, fmt.Errorf("error PEM decoding %s", filename)
	}

	return precertPEMBlock.Bytes, nil
}

func TestMismatches(t *testing.T) {
	now := time.Now()

	issuerKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	// A separate issuer key, used for signing the final certificate, but
	// using the same simulated issuer certificate.
	untrustedIssuerKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	subscriberKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	// By reading the crypto/x509 code, we know that Subject is the only field
	// of the issuer certificate that we need to care about for the purposes
	// of signing below.
	issuer := x509.Certificate{
		Subject: pkix.Name{
			CommonName: "Some Issuer",
		},
	}

	precertTemplate := x509.Certificate{
		SerialNumber: big.NewInt(3141592653589793238),
		NotBefore:    now,
		NotAfter:     now.Add(24 * time.Hour),
		DNSNames:     []string{"example.com"},
		ExtraExtensions: []pkix.Extension{
			{
				Id:    poisonOID,
				Value: []byte{0x5, 0x0},
			},
		},
	}

	precertDER, err := x509.CreateCertificate(rand.Reader, &precertTemplate, &issuer, &subscriberKey.PublicKey, issuerKey)
	if err != nil {
		t.Fatal(err)
	}

	// Sign a final certificate with the untrustedIssuerKey, first applying the
	// given modify function to the default template. Return the DER encoded bytes.
	makeFinalCert := func(modify func(c *x509.Certificate)) []byte {
		t.Helper()
		finalCertTemplate := &x509.Certificate{
			SerialNumber: big.NewInt(3141592653589793238),
			NotBefore:    now,
			NotAfter:     now.Add(24 * time.Hour),
			DNSNames:     []string{"example.com"},
			ExtraExtensions: []pkix.Extension{
				{
					Id:    sctListOID,
					Value: nil,
				},
			},
		}

		modify(finalCertTemplate)

		finalCertDER, err := x509.CreateCertificate(rand.Reader, finalCertTemplate,
			&issuer, &subscriberKey.PublicKey, untrustedIssuerKey)
		if err != nil {
			t.Fatal(err)
		}

		return finalCertDER
	}

	// Expect success with a matching precert and final cert
	finalCertDER := makeFinalCert(func(c *x509.Certificate) {})
	err = Correspond(precertDER, finalCertDER)
	if err != nil {
		t.Errorf("expected precert and final cert to correspond, got: %s", err)
	}

	// Set up a precert / final cert pair where the SCTList and poison extensions are
	// not in the same position
	precertTemplate2 := x509.Certificate{
		SerialNumber: big.NewInt(3141592653589793238),
		NotBefore:    now,
		NotAfter:     now.Add(24 * time.Hour),
		DNSNames:     []string{"example.com"},
		ExtraExtensions: []pkix.Extension{
			{
				Id:    poisonOID,
				Value: []byte{0x5, 0x0},
			},
			// Arbitrary extension to make poisonOID not be the last extension
			{
				Id:    []int{1, 2, 3, 4},
				Value: []byte{0x5, 0x0},
			},
		},
	}

	precertDER2, err := x509.CreateCertificate(rand.Reader, &precertTemplate2, &issuer, &subscriberKey.PublicKey, issuerKey)
	if err != nil {
		t.Fatal(err)
	}

	finalCertDER = makeFinalCert(func(c *x509.Certificate) {
		c.ExtraExtensions = []pkix.Extension{
			{
				Id:    []int{1, 2, 3, 4},
				Value: []byte{0x5, 0x0},
			},
			{
				Id:    sctListOID,
				Value: nil,
			},
		}
	})
	err = Correspond(precertDER2, finalCertDER)
	if err != nil {
		t.Errorf("expected precert and final cert to correspond with differently positioned extensions, got: %s", err)
	}

	// Expect failure with a mismatched Issuer
	issuer = x509.Certificate{
		Subject: pkix.Name{
			CommonName: "Some Other Issuer",
		},
	}

	finalCertDER = makeFinalCert(func(c *x509.Certificate) {})
	err = Correspond(precertDER, finalCertDER)
	if err == nil {
		t.Errorf("expected error for mismatched issuer, got nil error")
	}

	// Restore original issuer
	issuer = x509.Certificate{
		Subject: pkix.Name{
			CommonName: "Some Issuer",
		},
	}

	// Expect failure with a mismatched Serial
	finalCertDER = makeFinalCert(func(c *x509.Certificate) {
		c.SerialNumber = big.NewInt(2718281828459045)
	})
	err = Correspond(precertDER, finalCertDER)
	if err == nil {
		t.Errorf("expected error for mismatched serial, got nil error")
	}

	// Expect failure with mismatched names
	finalCertDER = makeFinalCert(func(c *x509.Certificate) {
		c.DNSNames = []string{"example.com", "www.example.com"}
	})

	err = Correspond(precertDER, finalCertDER)
	if err == nil {
		t.Errorf("expected error for mismatched names, got nil error")
	}

	// Expect failure with mismatched NotBefore
	finalCertDER = makeFinalCert(func(c *x509.Certificate) {
		c.NotBefore = now.Add(24 * time.Hour)
	})

	err = Correspond(precertDER, finalCertDER)
	if err == nil {
		t.Errorf("expected error for mismatched NotBefore, got nil error")
	}

	// Expect failure with mismatched NotAfter
	finalCertDER = makeFinalCert(func(c *x509.Certificate) {
		c.NotAfter = now.Add(48 * time.Hour)
	})
	err = Correspond(precertDER, finalCertDER)
	if err == nil {
		t.Errorf("expected error for mismatched NotAfter, got nil error")
	}

	// Expect failure for mismatched extensions
	finalCertDER = makeFinalCert(func(c *x509.Certificate) {
		c.ExtraExtensions = append(c.ExtraExtensions, pkix.Extension{
			Critical: true,
			Id:       []int{1, 2, 3},
			Value:    []byte("hello"),
		})
	})

	err = Correspond(precertDER, finalCertDER)
	if err == nil {
		t.Errorf("expected error for mismatched extensions, got nil error")
	}
	expectedError := "precert extension 2 () not equal to final cert extension 2 (06022a030101ff040568656c6c6f)"
	if err.Error() != expectedError {
		t.Errorf("expected error %q, got %q", expectedError, err)
	}
}

func TestUnwrapExtensions(t *testing.T) {
	validExtensionsOuter := []byte{0xA3, 0x3, 0x30, 0x1, 0x0}
	_, err := unwrapExtensions(validExtensionsOuter)
	if err != nil {
		t.Errorf("expected success for validExtensionsOuter, got %s", err)
	}

	invalidExtensionsOuter := []byte{0xA3, 0x99, 0x30, 0x1, 0x0}
	_, err = unwrapExtensions(invalidExtensionsOuter)
	if err == nil {
		t.Error("expected error for invalidExtensionsOuter, got none")
	}

	invalidExtensionsInner := []byte{0xA3, 0x3, 0x30, 0x99, 0x0}
	_, err = unwrapExtensions(invalidExtensionsInner)
	if err == nil {
		t.Error("expected error for invalidExtensionsInner, got none")
	}
}

func TestTBSFromCertDER(t *testing.T) {
	validCertOuter := []byte{0x30, 0x3, 0x30, 0x1, 0x0}
	_, err := tbsDERFromCertDER(validCertOuter)
	if err != nil {
		t.Errorf("expected success for validCertOuter, got %s", err)
	}

	invalidCertOuter := []byte{0x30, 0x99, 0x30, 0x1, 0x0}
	_, err = tbsDERFromCertDER(invalidCertOuter)
	if err == nil {
		t.Error("expected error for invalidCertOuter, got none")
	}

	invalidCertInner := []byte{0x30, 0x3, 0x30, 0x99, 0x0}
	_, err = tbsDERFromCertDER(invalidCertInner)
	if err == nil {
		t.Error("expected error for invalidExtensionsInner, got none")
	}
}
