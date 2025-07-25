package main

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/letsencrypt/boulder/pkcs11helpers"
	"github.com/letsencrypt/boulder/test"
	"github.com/miekg/pkcs11"
)

func setupCtx() pkcs11helpers.MockCtx {
	return pkcs11helpers.MockCtx{
		GenerateKeyPairFunc: func(pkcs11.SessionHandle, []*pkcs11.Mechanism, []*pkcs11.Attribute, []*pkcs11.Attribute) (pkcs11.ObjectHandle, pkcs11.ObjectHandle, error) {
			return 0, 0, nil
		},
		SignInitFunc: func(pkcs11.SessionHandle, []*pkcs11.Mechanism, pkcs11.ObjectHandle) error {
			return nil
		},
		GenerateRandomFunc: func(pkcs11.SessionHandle, int) ([]byte, error) {
			return []byte{1, 2, 3}, nil
		},
		FindObjectsInitFunc: func(pkcs11.SessionHandle, []*pkcs11.Attribute) error {
			return nil
		},
		FindObjectsFunc: func(pkcs11.SessionHandle, int) ([]pkcs11.ObjectHandle, bool, error) {
			return nil, false, nil
		},
		FindObjectsFinalFunc: func(pkcs11.SessionHandle) error {
			return nil
		},
	}
}

func TestGenerateKeyRSA(t *testing.T) {
	tmp := t.TempDir()

	ctx := setupCtx()
	rsaPriv, err := rsa.GenerateKey(rand.Reader, 1024)
	test.AssertNotError(t, err, "Failed to generate a test RSA key")
	ctx.GetAttributeValueFunc = func(pkcs11.SessionHandle, pkcs11.ObjectHandle, []*pkcs11.Attribute) ([]*pkcs11.Attribute, error) {
		return []*pkcs11.Attribute{
			pkcs11.NewAttribute(pkcs11.CKA_PUBLIC_EXPONENT, big.NewInt(int64(rsaPriv.E)).Bytes()),
			pkcs11.NewAttribute(pkcs11.CKA_MODULUS, rsaPriv.N.Bytes()),
		}, nil
	}
	ctx.SignFunc = func(_ pkcs11.SessionHandle, msg []byte) ([]byte, error) {
		// Chop of the hash identifier and feed back into rsa.SignPKCS1v15
		return rsa.SignPKCS1v15(rand.Reader, rsaPriv, crypto.SHA256, msg[19:])
	}
	s := &pkcs11helpers.Session{Module: &ctx, Session: 0}
	keyPath := path.Join(tmp, "test-rsa-key.pem")
	keyInfo, err := generateKey(s, "", keyPath, keyGenConfig{
		Type:         "rsa",
		RSAModLength: 1024,
	})
	test.AssertNotError(t, err, "Failed to generate RSA key")
	diskKeyBytes, err := os.ReadFile(keyPath)
	test.AssertNotError(t, err, "Failed to load key from disk")
	block, _ := pem.Decode(diskKeyBytes)
	diskKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	test.AssertNotError(t, err, "Failed to parse disk key")
	test.AssertDeepEquals(t, diskKey, keyInfo.key)
}

func setECGenerateFuncs(ctx *pkcs11helpers.MockCtx) {
	ecPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	ctx.GetAttributeValueFunc = func(pkcs11.SessionHandle, pkcs11.ObjectHandle, []*pkcs11.Attribute) ([]*pkcs11.Attribute, error) {
		return []*pkcs11.Attribute{
			pkcs11.NewAttribute(pkcs11.CKA_EC_PARAMS, []byte{6, 8, 42, 134, 72, 206, 61, 3, 1, 7}),
			pkcs11.NewAttribute(pkcs11.CKA_EC_POINT, elliptic.Marshal(elliptic.P256(), ecPriv.X, ecPriv.Y)),
		}, nil
	}
	ctx.SignFunc = func(_ pkcs11.SessionHandle, msg []byte) ([]byte, error) {
		return ecPKCS11Sign(ecPriv, msg)
	}
}

func TestGenerateKeyEC(t *testing.T) {
	tmp := t.TempDir()

	ctx := setupCtx()
	setECGenerateFuncs(&ctx)
	keyPath := path.Join(tmp, "test-ecdsa-key.pem")
	s := &pkcs11helpers.Session{Module: &ctx, Session: 0}
	keyInfo, err := generateKey(s, "", keyPath, keyGenConfig{
		Type:       "ecdsa",
		ECDSACurve: "P-256",
	})
	test.AssertNotError(t, err, "Failed to generate ECDSA key")
	diskKeyBytes, err := os.ReadFile(keyPath)
	test.AssertNotError(t, err, "Failed to load key from disk")
	block, _ := pem.Decode(diskKeyBytes)
	diskKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	test.AssertNotError(t, err, "Failed to parse disk key")
	test.AssertDeepEquals(t, diskKey, keyInfo.key)
}

func setFindObjectsFuncs(label string, ctx *pkcs11helpers.MockCtx) {
	var objectsFound []pkcs11.ObjectHandle
	ctx.FindObjectsInitFunc = func(_ pkcs11.SessionHandle, template []*pkcs11.Attribute) error {
		for _, attr := range template {
			if attr.Type == pkcs11.CKA_LABEL && string(attr.Value) == label {
				objectsFound = []pkcs11.ObjectHandle{1}
			}
		}
		return nil
	}
	ctx.FindObjectsFunc = func(pkcs11.SessionHandle, int) ([]pkcs11.ObjectHandle, bool, error) {
		return objectsFound, false, nil
	}
	ctx.FindObjectsFinalFunc = func(pkcs11.SessionHandle) error {
		objectsFound = nil
		return nil
	}
}

func TestGenerateKeySlotHasSomethingWithLabel(t *testing.T) {
	tmp := t.TempDir()

	ctx := setupCtx()
	label := "someLabel"
	setFindObjectsFuncs(label, &ctx)
	keyPath := path.Join(tmp, "should-not-exist.pem")
	s := &pkcs11helpers.Session{Module: &ctx, Session: 0}
	_, err := generateKey(s, label, keyPath, keyGenConfig{
		Type:       "ecdsa",
		ECDSACurve: "P-256",
	})
	test.AssertError(t, err, "expected failure for a slot with an object already in it")
	test.Assert(t, strings.HasPrefix(err.Error(), "expected no preexisting objects with label"), "wrong error")
}

func TestGenerateKeySlotHasSomethingWithDifferentLabel(t *testing.T) {
	tmp := t.TempDir()

	ctx := setupCtx()
	setECGenerateFuncs(&ctx)
	setFindObjectsFuncs("someLabel", &ctx)
	keyPath := path.Join(tmp, "should-not-exist.pem")
	s := &pkcs11helpers.Session{Module: &ctx, Session: 0}
	_, err := generateKey(s, "someOtherLabel", keyPath, keyGenConfig{
		Type:       "ecdsa",
		ECDSACurve: "P-256",
	})
	test.AssertNotError(t, err, "expected success even though there was an object with a different label")
}
