package privatekey

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"hash"
	"os"
)

func makeVerifyHash() (hash.Hash, error) {
	randBytes := make([]byte, 32)
	_, err := rand.Read(randBytes)
	if err != nil {
		return nil, err
	}

	hash := sha256.New()
	_, err = hash.Write(randBytes)
	if err != nil {
		return nil, err
	}
	return hash, nil
}

// verifyRSA is broken out of Verify for testing purposes.
func verifyRSA(privKey *rsa.PrivateKey, pubKey *rsa.PublicKey, msgHash hash.Hash) (crypto.Signer, crypto.PublicKey, error) {
	signatureRSA, err := rsa.SignPSS(rand.Reader, privKey, crypto.SHA256, msgHash.Sum(nil), nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to sign using the provided RSA private key: %s", err)
	}

	err = rsa.VerifyPSS(pubKey, crypto.SHA256, msgHash.Sum(nil), signatureRSA, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("the provided RSA private key failed signature verification: %s", err)
	}
	return privKey, privKey.Public(), nil
}

// verifyECDSA is broken out of Verify for testing purposes.
func verifyECDSA(privKey *ecdsa.PrivateKey, pubKey *ecdsa.PublicKey, msgHash hash.Hash) (crypto.Signer, crypto.PublicKey, error) {
	r, s, err := ecdsa.Sign(rand.Reader, privKey, msgHash.Sum(nil))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to sign using the provided ECDSA private key: %s", err)
	}

	verify := ecdsa.Verify(pubKey, msgHash.Sum(nil), r, s)
	if !verify {
		return nil, nil, errors.New("the provided ECDSA private key failed signature verification")
	}
	return privKey, privKey.Public(), nil
}

// verify ensures that the embedded PublicKey of the provided privateKey is
// actually a match for the private key. For an example of private keys
// embedding a mismatched public key, see:
// https://blog.hboeck.de/archives/888-How-I-tricked-Symantec-with-a-Fake-Private-Key.html.
func verify(privateKey crypto.Signer) (crypto.Signer, crypto.PublicKey, error) {
	verifyHash, err := makeVerifyHash()
	if err != nil {
		return nil, nil, err
	}

	switch k := privateKey.(type) {
	case *rsa.PrivateKey:
		return verifyRSA(k, &k.PublicKey, verifyHash)

	case *ecdsa.PrivateKey:
		return verifyECDSA(k, &k.PublicKey, verifyHash)

	default:
		// This should never happen.
		return nil, nil, errors.New("the provided private key could not be asserted to ECDSA or RSA")
	}
}

// Load decodes and parses a private key from the provided file path and returns
// the private key as crypto.Signer. keyPath is expected to be a PEM formatted
// RSA or ECDSA private key in a PKCS #1, PKCS# 8, or SEC 1 container. The
// embedded PublicKey of the provided private key will be verified as an actual
// match for the private key and returned as a crypto.PublicKey. This function
// is only intended for use in administrative tooling and tests.
func Load(keyPath string) (crypto.Signer, crypto.PublicKey, error) {
	keyBytes, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, nil, fmt.Errorf("could not read key file %q", keyPath)
	}

	var keyDER *pem.Block
	for {
		keyDER, keyBytes = pem.Decode(keyBytes)
		if keyDER == nil || keyDER.Type != "EC PARAMETERS" {
			break
		}
	}
	if keyDER == nil {
		return nil, nil, fmt.Errorf("no PEM formatted block found in %q", keyPath)
	}

	// Attempt to parse the PEM block as a private key in a PKCS #8 container.
	signer, err := x509.ParsePKCS8PrivateKey(keyDER.Bytes)
	if err == nil {
		cryptoSigner, ok := signer.(crypto.Signer)
		if ok {
			return verify(cryptoSigner)
		}
	}

	// Attempt to parse the PEM block as a private key in a PKCS #1 container.
	rsaSigner, err := x509.ParsePKCS1PrivateKey(keyDER.Bytes)
	if err != nil && keyDER.Type == "RSA PRIVATE KEY" {
		return nil, nil, fmt.Errorf("unable to parse %q as a PKCS#1 RSA private key: %w", keyPath, err)
	}
	if err == nil {
		return verify(rsaSigner)
	}

	// Attempt to parse the PEM block as a private key in a SEC 1 container.
	ecdsaSigner, err := x509.ParseECPrivateKey(keyDER.Bytes)
	if err == nil {
		return verify(ecdsaSigner)
	}
	return nil, nil, fmt.Errorf("unable to parse %q as a private key", keyPath)
}
