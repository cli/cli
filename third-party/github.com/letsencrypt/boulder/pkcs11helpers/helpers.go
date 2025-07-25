package pkcs11helpers

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"encoding/asn1"
	"errors"
	"fmt"
	"io"
	"math/big"

	"github.com/miekg/pkcs11"
)

type PKCtx interface {
	GenerateKeyPair(pkcs11.SessionHandle, []*pkcs11.Mechanism, []*pkcs11.Attribute, []*pkcs11.Attribute) (pkcs11.ObjectHandle, pkcs11.ObjectHandle, error)
	GetAttributeValue(pkcs11.SessionHandle, pkcs11.ObjectHandle, []*pkcs11.Attribute) ([]*pkcs11.Attribute, error)
	SignInit(pkcs11.SessionHandle, []*pkcs11.Mechanism, pkcs11.ObjectHandle) error
	Sign(pkcs11.SessionHandle, []byte) ([]byte, error)
	GenerateRandom(pkcs11.SessionHandle, int) ([]byte, error)
	FindObjectsInit(sh pkcs11.SessionHandle, temp []*pkcs11.Attribute) error
	FindObjects(sh pkcs11.SessionHandle, max int) ([]pkcs11.ObjectHandle, bool, error)
	FindObjectsFinal(sh pkcs11.SessionHandle) error
}

// Session represents a session with a given PKCS#11 module. It is not safe for
// concurrent access.
type Session struct {
	Module  PKCtx
	Session pkcs11.SessionHandle
}

func Initialize(module string, slot uint, pin string) (*Session, error) {
	ctx := pkcs11.New(module)
	if ctx == nil {
		return nil, errors.New("failed to load module")
	}
	err := ctx.Initialize()
	if err != nil {
		return nil, fmt.Errorf("couldn't initialize context: %s", err)
	}

	session, err := ctx.OpenSession(slot, pkcs11.CKF_SERIAL_SESSION|pkcs11.CKF_RW_SESSION)
	if err != nil {
		return nil, fmt.Errorf("couldn't open session: %s", err)
	}

	err = ctx.Login(session, pkcs11.CKU_USER, pin)
	if err != nil {
		return nil, fmt.Errorf("couldn't login: %s", err)
	}

	return &Session{ctx, session}, nil
}

// https://tools.ietf.org/html/rfc5759#section-3.2
var curveOIDs = map[string]asn1.ObjectIdentifier{
	"P-256": {1, 2, 840, 10045, 3, 1, 7},
	"P-384": {1, 3, 132, 0, 34},
}

// getPublicKeyID looks up the given public key in the PKCS#11 token, and
// returns its ID as a []byte, for use in looking up the corresponding private
// key.
func (s *Session) getPublicKeyID(label string, publicKey crypto.PublicKey) ([]byte, error) {
	var template []*pkcs11.Attribute
	switch key := publicKey.(type) {
	case *rsa.PublicKey:
		template = []*pkcs11.Attribute{
			pkcs11.NewAttribute(pkcs11.CKA_CLASS, pkcs11.CKO_PUBLIC_KEY),
			pkcs11.NewAttribute(pkcs11.CKA_LABEL, []byte(label)),
			pkcs11.NewAttribute(pkcs11.CKA_KEY_TYPE, pkcs11.CKK_RSA),
			pkcs11.NewAttribute(pkcs11.CKA_MODULUS, key.N.Bytes()),
			pkcs11.NewAttribute(pkcs11.CKA_PUBLIC_EXPONENT, big.NewInt(int64(key.E)).Bytes()),
		}
	case *ecdsa.PublicKey:
		// http://docs.oasis-open.org/pkcs11/pkcs11-curr/v2.40/os/pkcs11-curr-v2.40-os.html#_ftn1
		// PKCS#11 v2.20 specified that the CKA_EC_POINT was to be store in a DER-encoded
		// OCTET STRING.
		rawValue := asn1.RawValue{
			Tag:   asn1.TagOctetString,
			Bytes: elliptic.Marshal(key.Curve, key.X, key.Y),
		}
		marshalledPoint, err := asn1.Marshal(rawValue)
		if err != nil {
			return nil, err
		}
		curveOID, err := asn1.Marshal(curveOIDs[key.Curve.Params().Name])
		if err != nil {
			return nil, err
		}
		template = []*pkcs11.Attribute{
			pkcs11.NewAttribute(pkcs11.CKA_CLASS, pkcs11.CKO_PUBLIC_KEY),
			pkcs11.NewAttribute(pkcs11.CKA_LABEL, []byte(label)),
			pkcs11.NewAttribute(pkcs11.CKA_KEY_TYPE, pkcs11.CKK_EC),
			pkcs11.NewAttribute(pkcs11.CKA_EC_PARAMS, curveOID),
			pkcs11.NewAttribute(pkcs11.CKA_EC_POINT, marshalledPoint),
		}
	default:
		return nil, fmt.Errorf("unsupported public key of type %T", publicKey)
	}

	publicKeyHandle, err := s.FindObject(template)
	if err != nil {
		return nil, err
	}

	attrs, err := s.Module.GetAttributeValue(s.Session, publicKeyHandle, []*pkcs11.Attribute{
		pkcs11.NewAttribute(pkcs11.CKA_ID, nil),
	})
	if err != nil {
		return nil, err
	}
	if len(attrs) == 1 && attrs[0].Type == pkcs11.CKA_ID {
		return attrs[0].Value, nil
	}
	return nil, fmt.Errorf("invalid result from GetAttributeValue")
}

// getPrivateKey gets a handle to the private key whose CKA_ID matches the
// provided publicKeyID.
func (s *Session) getPrivateKey(publicKeyID []byte) (pkcs11.ObjectHandle, error) {
	return s.FindObject([]*pkcs11.Attribute{
		pkcs11.NewAttribute(pkcs11.CKA_CLASS, pkcs11.CKO_PRIVATE_KEY),
		pkcs11.NewAttribute(pkcs11.CKA_ID, publicKeyID),
	})
}

func (s *Session) GetAttributeValue(object pkcs11.ObjectHandle, attributes []*pkcs11.Attribute) ([]*pkcs11.Attribute, error) {
	return s.Module.GetAttributeValue(s.Session, object, attributes)
}

func (s *Session) GenerateKeyPair(m []*pkcs11.Mechanism, pubAttrs []*pkcs11.Attribute, privAttrs []*pkcs11.Attribute) (pkcs11.ObjectHandle, pkcs11.ObjectHandle, error) {
	return s.Module.GenerateKeyPair(s.Session, m, pubAttrs, privAttrs)
}

func (s *Session) GetRSAPublicKey(object pkcs11.ObjectHandle) (*rsa.PublicKey, error) {
	// Retrieve the public exponent and modulus for the public key
	attrs, err := s.Module.GetAttributeValue(s.Session, object, []*pkcs11.Attribute{
		pkcs11.NewAttribute(pkcs11.CKA_PUBLIC_EXPONENT, nil),
		pkcs11.NewAttribute(pkcs11.CKA_MODULUS, nil),
	})
	if err != nil {
		return nil, fmt.Errorf("Failed to retrieve key attributes: %s", err)
	}

	// Attempt to build the public key from the retrieved attributes
	pubKey := &rsa.PublicKey{}
	gotMod, gotExp := false, false
	for _, a := range attrs {
		switch a.Type {
		case pkcs11.CKA_PUBLIC_EXPONENT:
			pubKey.E = int(big.NewInt(0).SetBytes(a.Value).Int64())
			gotExp = true
		case pkcs11.CKA_MODULUS:
			pubKey.N = big.NewInt(0).SetBytes(a.Value)
			gotMod = true
		}
	}
	// Fail if we are missing either the public exponent or modulus
	if !gotExp || !gotMod {
		return nil, errors.New("Couldn't retrieve modulus and exponent")
	}
	return pubKey, nil
}

// oidDERToCurve maps the hex of the DER encoding of the various curve OIDs to
// the relevant curve parameters
var oidDERToCurve = map[string]elliptic.Curve{
	"06052B81040021":       elliptic.P224(),
	"06082A8648CE3D030107": elliptic.P256(),
	"06052B81040022":       elliptic.P384(),
	"06052B81040023":       elliptic.P521(),
}

func (s *Session) GetECDSAPublicKey(object pkcs11.ObjectHandle) (*ecdsa.PublicKey, error) {
	// Retrieve the curve and public point for the generated public key
	attrs, err := s.Module.GetAttributeValue(s.Session, object, []*pkcs11.Attribute{
		pkcs11.NewAttribute(pkcs11.CKA_EC_PARAMS, nil),
		pkcs11.NewAttribute(pkcs11.CKA_EC_POINT, nil),
	})
	if err != nil {
		return nil, fmt.Errorf("Failed to retrieve key attributes: %s", err)
	}

	pubKey := &ecdsa.PublicKey{}
	var pointBytes []byte
	for _, a := range attrs {
		switch a.Type {
		case pkcs11.CKA_EC_PARAMS:
			rCurve, present := oidDERToCurve[fmt.Sprintf("%X", a.Value)]
			if !present {
				return nil, errors.New("Unknown curve OID value returned")
			}
			pubKey.Curve = rCurve
		case pkcs11.CKA_EC_POINT:
			pointBytes = a.Value
		}
	}
	if pointBytes == nil || pubKey.Curve == nil {
		return nil, errors.New("Couldn't retrieve EC point and EC parameters")
	}

	x, y := elliptic.Unmarshal(pubKey.Curve, pointBytes)
	if x == nil {
		// http://docs.oasis-open.org/pkcs11/pkcs11-curr/v2.40/os/pkcs11-curr-v2.40-os.html#_ftn1
		// PKCS#11 v2.20 specified that the CKA_EC_POINT was to be stored in a DER-encoded
		// OCTET STRING.
		var point asn1.RawValue
		_, err = asn1.Unmarshal(pointBytes, &point)
		if err != nil {
			return nil, fmt.Errorf("Failed to unmarshal returned CKA_EC_POINT: %s", err)
		}
		if len(point.Bytes) == 0 {
			return nil, errors.New("Invalid CKA_EC_POINT value returned, OCTET string is empty")
		}
		x, y = elliptic.Unmarshal(pubKey.Curve, point.Bytes)
		if x == nil {
			return nil, errors.New("Invalid CKA_EC_POINT value returned, point is malformed")
		}
	}
	pubKey.X, pubKey.Y = x, y

	return pubKey, nil
}

type keyType int

const (
	RSAKey keyType = iota
	ECDSAKey
)

// Hash identifiers required for PKCS#11 RSA signing. Only support SHA-256, SHA-384,
// and SHA-512
var hashIdents = map[crypto.Hash][]byte{
	crypto.SHA256: {0x30, 0x31, 0x30, 0x0d, 0x06, 0x09, 0x60, 0x86, 0x48, 0x01, 0x65, 0x03, 0x04, 0x02, 0x01, 0x05, 0x00, 0x04, 0x20},
	crypto.SHA384: {0x30, 0x41, 0x30, 0x0d, 0x06, 0x09, 0x60, 0x86, 0x48, 0x01, 0x65, 0x03, 0x04, 0x02, 0x02, 0x05, 0x00, 0x04, 0x30},
	crypto.SHA512: {0x30, 0x51, 0x30, 0x0d, 0x06, 0x09, 0x60, 0x86, 0x48, 0x01, 0x65, 0x03, 0x04, 0x02, 0x03, 0x05, 0x00, 0x04, 0x40},
}

func (s *Session) Sign(object pkcs11.ObjectHandle, keyType keyType, digest []byte, hash crypto.Hash) ([]byte, error) {
	if len(digest) != hash.Size() {
		return nil, errors.New("digest length doesn't match hash length")
	}

	mech := make([]*pkcs11.Mechanism, 1)
	switch keyType {
	case RSAKey:
		mech[0] = pkcs11.NewMechanism(pkcs11.CKM_RSA_PKCS, nil)
		prefix, ok := hashIdents[hash]
		if !ok {
			return nil, errors.New("unsupported hash function")
		}
		digest = append(prefix, digest...)
	case ECDSAKey:
		mech[0] = pkcs11.NewMechanism(pkcs11.CKM_ECDSA, nil)
	}

	err := s.Module.SignInit(s.Session, mech, object)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize signing operation: %s", err)
	}
	signature, err := s.Module.Sign(s.Session, digest)
	if err != nil {
		return nil, fmt.Errorf("failed to sign data: %s", err)
	}

	return signature, nil
}

var ErrNoObject = errors.New("no objects found matching provided template")

// FindObject looks up a PKCS#11 object handle based on the provided template.
// In the case where zero or more than one objects are found to match the
// template an error is returned.
func (s *Session) FindObject(tmpl []*pkcs11.Attribute) (pkcs11.ObjectHandle, error) {
	err := s.Module.FindObjectsInit(s.Session, tmpl)
	if err != nil {
		return 0, err
	}
	handles, _, err := s.Module.FindObjects(s.Session, 2)
	if err != nil {
		return 0, err
	}
	err = s.Module.FindObjectsFinal(s.Session)
	if err != nil {
		return 0, err
	}
	if len(handles) == 0 {
		return 0, ErrNoObject
	}
	if len(handles) > 1 {
		return 0, fmt.Errorf("too many objects (%d) that match the provided template", len(handles))
	}
	return handles[0], nil
}

// x509Signer is a convenience wrapper used for converting between the
// PKCS#11 ECDSA signature format and the RFC 5480 one which is required
// for X.509 certificates
type x509Signer struct {
	session      *Session
	objectHandle pkcs11.ObjectHandle
	keyType      keyType

	pub crypto.PublicKey
}

// Sign signs a digest. If the signing key is ECDSA then the signature
// is converted from the PKCS#11 format to the RFC 5480 format. For RSA keys a
// conversion step is not needed.
func (p *x509Signer) Sign(rand io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	signature, err := p.session.Sign(p.objectHandle, p.keyType, digest, opts.HashFunc())
	if err != nil {
		return nil, err
	}

	if p.keyType == ECDSAKey {
		// Convert from the PKCS#11 format to the RFC 5480 format so that
		// it can be used in a X.509 certificate
		r := big.NewInt(0).SetBytes(signature[:len(signature)/2])
		s := big.NewInt(0).SetBytes(signature[len(signature)/2:])
		signature, err = asn1.Marshal(struct {
			R, S *big.Int
		}{R: r, S: s})
		if err != nil {
			return nil, fmt.Errorf("failed to convert signature to RFC 5480 format: %s", err)
		}
	}
	return signature, nil
}

func (p *x509Signer) Public() crypto.PublicKey {
	return p.pub
}

// NewSigner constructs an x509Signer for the private key object associated with the
// given label and public key.
func (s *Session) NewSigner(label string, publicKey crypto.PublicKey) (crypto.Signer, error) {
	var kt keyType
	switch publicKey.(type) {
	case *rsa.PublicKey:
		kt = RSAKey
	case *ecdsa.PublicKey:
		kt = ECDSAKey
	default:
		return nil, fmt.Errorf("unsupported public key of type %T", publicKey)
	}

	publicKeyID, err := s.getPublicKeyID(label, publicKey)
	if err != nil {
		return nil, fmt.Errorf("looking up public key: %s", err)
	}

	// Fetch the private key by matching its id to the public key handle.
	privateKeyHandle, err := s.getPrivateKey(publicKeyID)
	if err != nil {
		return nil, fmt.Errorf("getting private key: %s", err)
	}
	return &x509Signer{
		session:      s,
		objectHandle: privateKeyHandle,
		keyType:      kt,
		pub:          publicKey,
	}, nil
}

func NewMock() *MockCtx {
	return &MockCtx{}
}

func NewSessionWithMock() (*Session, *MockCtx) {
	ctx := NewMock()
	return &Session{ctx, 0}, ctx
}

type MockCtx struct {
	GenerateKeyPairFunc   func(pkcs11.SessionHandle, []*pkcs11.Mechanism, []*pkcs11.Attribute, []*pkcs11.Attribute) (pkcs11.ObjectHandle, pkcs11.ObjectHandle, error)
	GetAttributeValueFunc func(pkcs11.SessionHandle, pkcs11.ObjectHandle, []*pkcs11.Attribute) ([]*pkcs11.Attribute, error)
	SignInitFunc          func(pkcs11.SessionHandle, []*pkcs11.Mechanism, pkcs11.ObjectHandle) error
	SignFunc              func(pkcs11.SessionHandle, []byte) ([]byte, error)
	GenerateRandomFunc    func(pkcs11.SessionHandle, int) ([]byte, error)
	FindObjectsInitFunc   func(sh pkcs11.SessionHandle, temp []*pkcs11.Attribute) error
	FindObjectsFunc       func(sh pkcs11.SessionHandle, max int) ([]pkcs11.ObjectHandle, bool, error)
	FindObjectsFinalFunc  func(sh pkcs11.SessionHandle) error
}

func (mc MockCtx) GenerateKeyPair(s pkcs11.SessionHandle, m []*pkcs11.Mechanism, a1 []*pkcs11.Attribute, a2 []*pkcs11.Attribute) (pkcs11.ObjectHandle, pkcs11.ObjectHandle, error) {
	return mc.GenerateKeyPairFunc(s, m, a1, a2)
}

func (mc MockCtx) GetAttributeValue(s pkcs11.SessionHandle, o pkcs11.ObjectHandle, a []*pkcs11.Attribute) ([]*pkcs11.Attribute, error) {
	return mc.GetAttributeValueFunc(s, o, a)
}

func (mc MockCtx) SignInit(s pkcs11.SessionHandle, m []*pkcs11.Mechanism, o pkcs11.ObjectHandle) error {
	return mc.SignInitFunc(s, m, o)
}

func (mc MockCtx) Sign(s pkcs11.SessionHandle, m []byte) ([]byte, error) {
	return mc.SignFunc(s, m)
}

func (mc MockCtx) GenerateRandom(s pkcs11.SessionHandle, c int) ([]byte, error) {
	return mc.GenerateRandomFunc(s, c)
}

func (mc MockCtx) FindObjectsInit(sh pkcs11.SessionHandle, temp []*pkcs11.Attribute) error {
	return mc.FindObjectsInitFunc(sh, temp)
}

func (mc MockCtx) FindObjects(sh pkcs11.SessionHandle, max int) ([]pkcs11.ObjectHandle, bool, error) {
	return mc.FindObjectsFunc(sh, max)
}

func (mc MockCtx) FindObjectsFinal(sh pkcs11.SessionHandle) error {
	return mc.FindObjectsFinalFunc(sh)
}
