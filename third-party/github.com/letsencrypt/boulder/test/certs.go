package test

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"testing"
	"time"

	"github.com/jmhodges/clock"
)

// LoadSigner loads a PEM private key specified by filename or returns an error.
// Can be paired with issuance.LoadCertificate to get both a CA cert and its
// associated private key for use in signing throwaway test certs.
func LoadSigner(filename string) (crypto.Signer, error) {
	keyBytes, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	// pem.Decode does not return an error as its 2nd arg, but instead the "rest"
	// that was leftover from parsing the PEM block. We only care if the decoded
	// PEM block was empty for this test function.
	block, _ := pem.Decode(keyBytes)
	if block == nil {
		return nil, errors.New("Unable to decode private key PEM bytes")
	}

	// Try decoding as an RSA private key
	if rsaKey, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return rsaKey, nil
	}

	// Try decoding as a PKCS8 private key
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		// Determine the key's true type and return it as a crypto.Signer
		switch k := key.(type) {
		case *rsa.PrivateKey:
			return k, nil
		case *ecdsa.PrivateKey:
			return k, nil
		}
	}

	// Try as an ECDSA private key
	if ecdsaKey, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
		return ecdsaKey, nil
	}

	// Nothing worked! Fail hard.
	return nil, errors.New("Unable to decode private key PEM bytes")
}

// ThrowAwayCert is a small test helper function that creates a self-signed
// certificate with one SAN. It returns the parsed certificate and its serial
// in string form for convenience.
// The certificate returned from this function is the bare minimum needed for
// most tests and isn't a robust example of a complete end entity certificate.
func ThrowAwayCert(t *testing.T, clk clock.Clock) (string, *x509.Certificate) {
	var nameBytes [3]byte
	_, _ = rand.Read(nameBytes[:])
	name := fmt.Sprintf("%s.example.com", hex.EncodeToString(nameBytes[:]))

	// Generate a random IPv6 address under the RFC 3849 space.
	// https://www.rfc-editor.org/rfc/rfc3849.txt
	var ipBytes [12]byte
	_, _ = rand.Read(ipBytes[:])
	ipPrefix, _ := hex.DecodeString("20010db8")
	ip := net.IP(bytes.Join([][]byte{ipPrefix, ipBytes[:]}, nil))

	var serialBytes [16]byte
	_, _ = rand.Read(serialBytes[:])
	serial := big.NewInt(0).SetBytes(serialBytes[:])

	key, err := ecdsa.GenerateKey(elliptic.P224(), rand.Reader)
	AssertNotError(t, err, "rsa.GenerateKey failed")

	template := &x509.Certificate{
		SerialNumber:          serial,
		DNSNames:              []string{name},
		IPAddresses:           []net.IP{ip},
		NotBefore:             clk.Now(),
		NotAfter:              clk.Now().Add(6 * 24 * time.Hour),
		IssuingCertificateURL: []string{"http://localhost:4001/acme/issuer-cert/1234"},
	}

	testCertDER, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	AssertNotError(t, err, "x509.CreateCertificate failed")
	testCert, err := x509.ParseCertificate(testCertDER)
	AssertNotError(t, err, "failed to parse self-signed cert DER")

	return fmt.Sprintf("%036x", serial), testCert
}
