package privatekey

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"testing"

	"github.com/letsencrypt/boulder/test"
)

func TestVerifyRSAKeyPair(t *testing.T) {
	privKey1, err := rsa.GenerateKey(rand.Reader, 2048)
	test.AssertNotError(t, err, "Failed while generating test key 1")

	_, _, err = verify(privKey1)
	test.AssertNotError(t, err, "Failed to verify valid key")

	privKey2, err := rsa.GenerateKey(rand.Reader, 2048)
	test.AssertNotError(t, err, "Failed while generating test key 2")

	verifyHash, err := makeVerifyHash()
	test.AssertNotError(t, err, "Failed to make verify hash: %s")

	_, _, err = verifyRSA(privKey1, &privKey2.PublicKey, verifyHash)
	test.AssertError(t, err, "Failed to detect invalid key pair")
}

func TestVerifyECDSAKeyPair(t *testing.T) {
	privKey1, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	test.AssertNotError(t, err, "Failed while generating test key 1")

	_, _, err = verify(privKey1)
	test.AssertNotError(t, err, "Failed to verify valid key")

	privKey2, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	test.AssertNotError(t, err, "Failed while generating test key 2")

	verifyHash, err := makeVerifyHash()
	test.AssertNotError(t, err, "Failed to make verify hash: %s")

	_, _, err = verifyECDSA(privKey1, &privKey2.PublicKey, verifyHash)
	test.AssertError(t, err, "Failed to detect invalid key pair")
}

func TestLoad(t *testing.T) {
	signer, public, err := Load("../test/hierarchy/ee-e1.key.pem")
	test.AssertNotError(t, err, "Failed to load a valid ECDSA key file")
	test.AssertNotNil(t, signer, "Signer should not be Nil")
	test.AssertNotNil(t, public, "Public should not be Nil")

	signer, public, err = Load("../test/hierarchy/ee-r3.key.pem")
	test.AssertNotError(t, err, "Failed to load a valid RSA key file")
	test.AssertNotNil(t, signer, "Signer should not be Nil")
	test.AssertNotNil(t, public, "Public should not be Nil")

	signer, public, err = Load("../test/hierarchy/ee-e1.cert.pem")
	test.AssertError(t, err, "Should have failed, file is a certificate")
	test.AssertNil(t, signer, "Signer should be nil")
	test.AssertNil(t, public, "Public should be nil")
}
