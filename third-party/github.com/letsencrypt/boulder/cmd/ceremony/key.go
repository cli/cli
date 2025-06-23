package main

import (
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"

	"github.com/letsencrypt/boulder/pkcs11helpers"
	"github.com/miekg/pkcs11"
)

type hsmRandReader struct {
	*pkcs11helpers.Session
}

func newRandReader(session *pkcs11helpers.Session) *hsmRandReader {
	return &hsmRandReader{session}
}

func (hrr hsmRandReader) Read(p []byte) (n int, err error) {
	r, err := hrr.Module.GenerateRandom(hrr.Session.Session, len(p))
	if err != nil {
		return 0, err
	}
	copy(p[:], r)
	return len(r), nil
}

type generateArgs struct {
	mechanism    []*pkcs11.Mechanism
	privateAttrs []*pkcs11.Attribute
	publicAttrs  []*pkcs11.Attribute
}

// keyInfo is a struct used to pass around information about the public key
// associated with the generated private key. der contains the DER encoding
// of the SubjectPublicKeyInfo structure for the public key. id contains the
// HSM key pair object ID.
type keyInfo struct {
	key crypto.PublicKey
	der []byte
	id  []byte
}

func generateKey(session *pkcs11helpers.Session, label string, outputPath string, config keyGenConfig) (*keyInfo, error) {
	_, err := session.FindObject([]*pkcs11.Attribute{
		{Type: pkcs11.CKA_LABEL, Value: []byte(label)},
	})
	if err != pkcs11helpers.ErrNoObject {
		return nil, fmt.Errorf("expected no preexisting objects with label %q in slot for key storage. got error: %s", label, err)
	}

	var pubKey crypto.PublicKey
	var keyID []byte
	switch config.Type {
	case "rsa":
		pubKey, keyID, err = rsaGenerate(session, label, config.RSAModLength)
		if err != nil {
			return nil, fmt.Errorf("failed to generate RSA key pair: %s", err)
		}
	case "ecdsa":
		pubKey, keyID, err = ecGenerate(session, label, config.ECDSACurve)
		if err != nil {
			return nil, fmt.Errorf("failed to generate ECDSA key pair: %s", err)
		}
	}

	der, err := x509.MarshalPKIXPublicKey(pubKey)
	if err != nil {
		return nil, fmt.Errorf("Failed to marshal public key: %s", err)
	}

	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
	log.Printf("Public key PEM:\n%s\n", pemBytes)
	err = writeFile(outputPath, pemBytes)
	if err != nil {
		return nil, fmt.Errorf("Failed to write public key to %q: %s", outputPath, err)
	}
	log.Printf("Public key written to %q\n", outputPath)

	return &keyInfo{key: pubKey, der: der, id: keyID}, nil
}
