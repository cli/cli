package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"testing"

	"github.com/letsencrypt/boulder/pkcs11helpers"
	"github.com/letsencrypt/boulder/test"
	"github.com/miekg/pkcs11"
)

func TestECPub(t *testing.T) {
	s, ctx := pkcs11helpers.NewSessionWithMock()

	// test we fail when pkcs11helpers.GetECDSAPublicKey fails
	ctx.GetAttributeValueFunc = func(pkcs11.SessionHandle, pkcs11.ObjectHandle, []*pkcs11.Attribute) ([]*pkcs11.Attribute, error) {
		return nil, errors.New("bad!")
	}
	_, err := ecPub(s, 0, elliptic.P256())
	test.AssertError(t, err, "ecPub didn't fail with non-matching curve")
	test.AssertEquals(t, err.Error(), "Failed to retrieve key attributes: bad!")

	// test we fail to construct key with non-matching curve
	ctx.GetAttributeValueFunc = func(pkcs11.SessionHandle, pkcs11.ObjectHandle, []*pkcs11.Attribute) ([]*pkcs11.Attribute, error) {
		return []*pkcs11.Attribute{
			pkcs11.NewAttribute(pkcs11.CKA_EC_PARAMS, []byte{6, 5, 43, 129, 4, 0, 33}),
			pkcs11.NewAttribute(pkcs11.CKA_EC_POINT, []byte{4, 217, 225, 246, 210, 153, 134, 246, 104, 95, 79, 122, 206, 135, 241, 37, 114, 199, 87, 56, 167, 83, 56, 136, 174, 6, 145, 97, 239, 221, 49, 67, 148, 13, 126, 65, 90, 208, 195, 193, 171, 105, 40, 98, 132, 124, 30, 189, 215, 197, 178, 226, 166, 238, 240, 57, 215}),
		}, nil
	}
	_, err = ecPub(s, 0, elliptic.P256())
	test.AssertError(t, err, "ecPub didn't fail with non-matching curve")
}

func TestECGenerate(t *testing.T) {
	ctx := pkcs11helpers.MockCtx{}
	s := &pkcs11helpers.Session{Module: &ctx, Session: 0}
	ctx.GenerateRandomFunc = func(pkcs11.SessionHandle, int) ([]byte, error) {
		return []byte{1, 2, 3}, nil
	}
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	test.AssertNotError(t, err, "Failed to generate a ECDSA test key")

	// Test ecGenerate fails with unknown curve
	_, _, err = ecGenerate(s, "", "bad-curve")
	test.AssertError(t, err, "ecGenerate accepted unknown curve")

	// Test ecGenerate fails when GenerateKeyPair fails
	ctx.GenerateKeyPairFunc = func(pkcs11.SessionHandle, []*pkcs11.Mechanism, []*pkcs11.Attribute, []*pkcs11.Attribute) (pkcs11.ObjectHandle, pkcs11.ObjectHandle, error) {
		return 0, 0, errors.New("bad")
	}
	_, _, err = ecGenerate(s, "", "P-256")
	test.AssertError(t, err, "ecGenerate didn't fail on GenerateKeyPair error")

	// Test ecGenerate fails when ecPub fails
	ctx.GenerateKeyPairFunc = func(pkcs11.SessionHandle, []*pkcs11.Mechanism, []*pkcs11.Attribute, []*pkcs11.Attribute) (pkcs11.ObjectHandle, pkcs11.ObjectHandle, error) {
		return 0, 0, nil
	}
	ctx.GetAttributeValueFunc = func(pkcs11.SessionHandle, pkcs11.ObjectHandle, []*pkcs11.Attribute) ([]*pkcs11.Attribute, error) {
		return nil, errors.New("bad")
	}
	_, _, err = ecGenerate(s, "", "P-256")
	test.AssertError(t, err, "ecGenerate didn't fail on ecPub error")

	// Test ecGenerate fails when ecVerify fails
	ctx.GetAttributeValueFunc = func(pkcs11.SessionHandle, pkcs11.ObjectHandle, []*pkcs11.Attribute) ([]*pkcs11.Attribute, error) {
		return []*pkcs11.Attribute{
			pkcs11.NewAttribute(pkcs11.CKA_EC_PARAMS, []byte{6, 8, 42, 134, 72, 206, 61, 3, 1, 7}),
			pkcs11.NewAttribute(pkcs11.CKA_EC_POINT, elliptic.Marshal(elliptic.P256(), priv.X, priv.Y)),
		}, nil
	}
	ctx.GenerateRandomFunc = func(pkcs11.SessionHandle, int) ([]byte, error) {
		return nil, errors.New("yup")
	}
	_, _, err = ecGenerate(s, "", "P-256")
	test.AssertError(t, err, "ecGenerate didn't fail on ecVerify error")

	// Test ecGenerate doesn't fail when everything works
	ctx.SignInitFunc = func(pkcs11.SessionHandle, []*pkcs11.Mechanism, pkcs11.ObjectHandle) error {
		return nil
	}
	ctx.GenerateRandomFunc = func(pkcs11.SessionHandle, int) ([]byte, error) {
		return []byte{1, 2, 3}, nil
	}
	ctx.SignFunc = func(_ pkcs11.SessionHandle, msg []byte) ([]byte, error) {
		return ecPKCS11Sign(priv, msg)
	}
	_, _, err = ecGenerate(s, "", "P-256")
	test.AssertNotError(t, err, "ecGenerate didn't succeed when everything worked as expected")
}

func ecPKCS11Sign(priv *ecdsa.PrivateKey, msg []byte) ([]byte, error) {
	r, s, err := ecdsa.Sign(rand.Reader, priv, msg[:])
	if err != nil {
		return nil, err
	}
	rBytes := r.Bytes()
	sBytes := s.Bytes()
	// http://docs.oasis-open.org/pkcs11/pkcs11-curr/v2.40/os/pkcs11-curr-v2.40-os.html
	// Section 2.3.1: EC Signatures
	// "If r and s have different octet length, the shorter of both must be padded with
	// leading zero octets such that both have the same octet length."
	switch {
	case len(rBytes) < len(sBytes):
		padding := make([]byte, len(sBytes)-len(rBytes))
		rBytes = append(padding, rBytes...)
	case len(rBytes) > len(sBytes):
		padding := make([]byte, len(rBytes)-len(sBytes))
		sBytes = append(padding, sBytes...)
	}
	return append(rBytes, sBytes...), nil
}
