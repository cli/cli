package issuance

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net"
	"sync"
	"time"

	ct "github.com/google/certificate-transparency-go"
	cttls "github.com/google/certificate-transparency-go/tls"
	ctx509 "github.com/google/certificate-transparency-go/x509"
	"github.com/jmhodges/clock"
	"github.com/zmap/zlint/v3/lint"

	"github.com/letsencrypt/boulder/cmd"
	"github.com/letsencrypt/boulder/config"
	"github.com/letsencrypt/boulder/linter"
	"github.com/letsencrypt/boulder/precert"
)

// ProfileConfig describes the certificate issuance constraints for all issuers.
type ProfileConfig struct {
	// AllowMustStaple, when false, causes all IssuanceRequests which specify the
	// OCSP Must Staple extension to be rejected.
	//
	// Deprecated: This has no effect, Must Staple is always omitted.
	// TODO(#8177): Remove this.
	AllowMustStaple bool

	// OmitCommonName causes the CN field to be excluded from the resulting
	// certificate, regardless of its inclusion in the IssuanceRequest.
	OmitCommonName bool
	// OmitKeyEncipherment causes the keyEncipherment bit to be omitted from the
	// Key Usage field of all certificates (instead of only from ECDSA certs).
	OmitKeyEncipherment bool
	// OmitClientAuth causes the id-kp-clientAuth OID (TLS Client Authentication)
	// to be omitted from the EKU extension.
	OmitClientAuth bool
	// OmitSKID causes the Subject Key Identifier extension to be omitted.
	OmitSKID bool
	// OmitOCSP causes the OCSP URI field to be omitted from the Authority
	// Information Access extension. This cannot be true unless
	// IncludeCRLDistributionPoints is also true, to ensure that every
	// certificate has at least one revocation mechanism included.
	//
	// Deprecated: This has no effect; OCSP is always omitted.
	// TODO(#8177): Remove this.
	OmitOCSP bool
	// IncludeCRLDistributionPoints causes the CRLDistributionPoints extension to
	// be added to all certificates issued by this profile.
	IncludeCRLDistributionPoints bool

	MaxValidityPeriod   config.Duration
	MaxValidityBackdate config.Duration

	// LintConfig is a path to a zlint config file, which can be used to control
	// the behavior of zlint's "customizable lints".
	LintConfig string
	// IgnoredLints is a list of lint names that we know will fail for this
	// profile, and which we know it is safe to ignore.
	IgnoredLints []string
}

// PolicyConfig describes a policy
type PolicyConfig struct {
	OID string `validate:"required"`
}

// Profile is the validated structure created by reading in ProfileConfigs and IssuerConfigs
type Profile struct {
	omitCommonName      bool
	omitKeyEncipherment bool
	omitClientAuth      bool
	omitSKID            bool

	includeCRLDistributionPoints bool

	maxBackdate time.Duration
	maxValidity time.Duration

	lints lint.Registry
}

// NewProfile converts the profile config into a usable profile.
func NewProfile(profileConfig *ProfileConfig) (*Profile, error) {
	// The Baseline Requirements, Section 7.1.2.7, says that the notBefore time
	// must be "within 48 hours of the time of signing". We can be even stricter.
	if profileConfig.MaxValidityBackdate.Duration >= 24*time.Hour {
		return nil, fmt.Errorf("backdate %q is too large", profileConfig.MaxValidityBackdate.Duration)
	}

	// Our CP/CPS, Section 7.1, says that our Subscriber Certificates have a
	// validity period of "up to 100 days".
	if profileConfig.MaxValidityPeriod.Duration >= 100*24*time.Hour {
		return nil, fmt.Errorf("validity period %q is too large", profileConfig.MaxValidityPeriod.Duration)
	}

	// Although the Baseline Requirements say that revocation information may be
	// omitted entirely *for short-lived certs*, the Microsoft root program still
	// requires that at least one revocation mechanism be included in all certs.
	// TODO(#7673): Remove this restriction.
	if !profileConfig.IncludeCRLDistributionPoints {
		return nil, fmt.Errorf("at least one revocation mechanism must be included")
	}

	lints, err := linter.NewRegistry(profileConfig.IgnoredLints)
	cmd.FailOnError(err, "Failed to create zlint registry")
	if profileConfig.LintConfig != "" {
		lintconfig, err := lint.NewConfigFromFile(profileConfig.LintConfig)
		cmd.FailOnError(err, "Failed to load zlint config file")
		lints.SetConfiguration(lintconfig)
	}

	sp := &Profile{
		omitCommonName:               profileConfig.OmitCommonName,
		omitKeyEncipherment:          profileConfig.OmitKeyEncipherment,
		omitClientAuth:               profileConfig.OmitClientAuth,
		omitSKID:                     profileConfig.OmitSKID,
		includeCRLDistributionPoints: profileConfig.IncludeCRLDistributionPoints,
		maxBackdate:                  profileConfig.MaxValidityBackdate.Duration,
		maxValidity:                  profileConfig.MaxValidityPeriod.Duration,
		lints:                        lints,
	}

	return sp, nil
}

// GenerateValidity returns a notBefore/notAfter pair bracketing the input time,
// based on the profile's configured backdate and validity.
func (p *Profile) GenerateValidity(now time.Time) (time.Time, time.Time) {
	// Don't use the full maxBackdate, to ensure that the actual backdate remains
	// acceptable throughout the rest of the issuance process.
	backdate := time.Duration(float64(p.maxBackdate.Nanoseconds()) * 0.9)
	notBefore := now.Add(-1 * backdate)
	// Subtract one second, because certificate validity periods are *inclusive*
	// of their final second (Baseline Requirements, Section 1.6.1).
	notAfter := notBefore.Add(p.maxValidity).Add(-1 * time.Second)
	return notBefore, notAfter
}

// requestValid verifies the passed IssuanceRequest against the profile. If the
// request doesn't match the signing profile an error is returned.
func (i *Issuer) requestValid(clk clock.Clock, prof *Profile, req *IssuanceRequest) error {
	switch req.PublicKey.PublicKey.(type) {
	case *rsa.PublicKey, *ecdsa.PublicKey:
	default:
		return errors.New("unsupported public key type")
	}

	if len(req.precertDER) == 0 && !i.active {
		return errors.New("inactive issuer cannot issue precert")
	}

	if len(req.SubjectKeyId) != 0 && len(req.SubjectKeyId) != 20 {
		return errors.New("unexpected subject key ID length")
	}

	if req.IncludeCTPoison && req.sctList != nil {
		return errors.New("cannot include both ct poison and sct list extensions")
	}

	// The validity period is calculated inclusive of the whole second represented
	// by the notAfter timestamp.
	validity := req.NotAfter.Add(time.Second).Sub(req.NotBefore)
	if validity <= 0 {
		return errors.New("NotAfter must be after NotBefore")
	}
	if validity > prof.maxValidity {
		return fmt.Errorf("validity period is more than the maximum allowed period (%s>%s)", validity, prof.maxValidity)
	}
	backdatedBy := clk.Now().Sub(req.NotBefore)
	if backdatedBy > prof.maxBackdate {
		return fmt.Errorf("NotBefore is backdated more than the maximum allowed period (%s>%s)", backdatedBy, prof.maxBackdate)
	}
	if backdatedBy < 0 {
		return errors.New("NotBefore is in the future")
	}

	// We use 19 here because a 20-byte serial could produce >20 octets when
	// encoded in ASN.1. That happens when the first byte is >0x80. See
	// https://letsencrypt.org/docs/a-warm-welcome-to-asn1-and-der/#integer-encoding
	if len(req.Serial) > 19 || len(req.Serial) < 9 {
		return errors.New("serial must be between 9 and 19 bytes")
	}

	return nil
}

// Baseline Requirements, Section 7.1.6.1: domain-validated
var domainValidatedOID = func() x509.OID {
	x509OID, err := x509.OIDFromInts([]uint64{2, 23, 140, 1, 2, 1})
	if err != nil {
		// This should never happen, as the OID is hardcoded.
		panic(fmt.Errorf("failed to create OID using ints %v: %s", x509OID, err))
	}
	return x509OID
}()

func (i *Issuer) generateTemplate() *x509.Certificate {
	template := &x509.Certificate{
		SignatureAlgorithm:    i.sigAlg,
		IssuingCertificateURL: []string{i.issuerURL},
		BasicConstraintsValid: true,
		// Baseline Requirements, Section 7.1.6.1: domain-validated
		Policies: []x509.OID{domainValidatedOID},
	}

	return template
}

var ctPoisonExt = pkix.Extension{
	// OID for CT poison, RFC 6962 (was never assigned a proper id-pe- name)
	Id:       asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 11129, 2, 4, 3},
	Value:    asn1.NullBytes,
	Critical: true,
}

// OID for SCT list, RFC 6962 (was never assigned a proper id-pe- name)
var sctListOID = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 11129, 2, 4, 2}

func generateSCTListExt(scts []ct.SignedCertificateTimestamp) (pkix.Extension, error) {
	list := ctx509.SignedCertificateTimestampList{}
	for _, sct := range scts {
		sctBytes, err := cttls.Marshal(sct)
		if err != nil {
			return pkix.Extension{}, err
		}
		list.SCTList = append(list.SCTList, ctx509.SerializedSCT{Val: sctBytes})
	}
	listBytes, err := cttls.Marshal(list)
	if err != nil {
		return pkix.Extension{}, err
	}
	extBytes, err := asn1.Marshal(listBytes)
	if err != nil {
		return pkix.Extension{}, err
	}
	return pkix.Extension{
		Id:    sctListOID,
		Value: extBytes,
	}, nil
}

// MarshalablePublicKey is a wrapper for crypto.PublicKey with a custom JSON
// marshaller that encodes the public key as a DER-encoded SubjectPublicKeyInfo.
type MarshalablePublicKey struct {
	crypto.PublicKey
}

func (pk MarshalablePublicKey) MarshalJSON() ([]byte, error) {
	keyDER, err := x509.MarshalPKIXPublicKey(pk.PublicKey)
	if err != nil {
		return nil, err
	}
	return json.Marshal(keyDER)
}

type HexMarshalableBytes []byte

func (h HexMarshalableBytes) MarshalJSON() ([]byte, error) {
	return json.Marshal(fmt.Sprintf("%x", h))
}

// IssuanceRequest describes a certificate issuance request
//
// It can be marshaled as JSON for logging purposes, though note that sctList and precertDER
// will be omitted from the marshaled output because they are unexported.
type IssuanceRequest struct {
	// PublicKey is of type MarshalablePublicKey so we can log an IssuanceRequest as a JSON object.
	PublicKey    MarshalablePublicKey
	SubjectKeyId HexMarshalableBytes

	Serial HexMarshalableBytes

	NotBefore time.Time
	NotAfter  time.Time

	CommonName  string
	DNSNames    []string
	IPAddresses []net.IP

	IncludeCTPoison bool

	// sctList is a list of SCTs to include in a final certificate.
	// If it is non-empty, PrecertDER must also be non-empty.
	sctList []ct.SignedCertificateTimestamp
	// precertDER is the encoded bytes of the precertificate that a
	// final certificate is expected to correspond to. If it is non-empty,
	// SCTList must also be non-empty.
	precertDER []byte
}

// An issuanceToken represents an assertion that Issuer.Lint has generated
// a linting certificate for a given input and run the linter over it with no
// errors. The token may be redeemed (at most once) to sign a certificate or
// precertificate with the same Issuer's private key, containing the same
// contents that were linted.
type issuanceToken struct {
	mu       sync.Mutex
	template *x509.Certificate
	pubKey   MarshalablePublicKey
	// A pointer to the issuer that created this token. This token may only
	// be redeemed by the same issuer.
	issuer *Issuer
}

// Prepare combines the given profile and request with the Issuer's information
// to create a template certificate. It then generates a linting certificate
// from that template and runs the linter over it. If successful, returns both
// the linting certificate (which can be stored) and an issuanceToken. The
// issuanceToken can be used to sign a matching certificate with this Issuer's
// private key.
func (i *Issuer) Prepare(prof *Profile, req *IssuanceRequest) ([]byte, *issuanceToken, error) {
	// check request is valid according to the issuance profile
	err := i.requestValid(i.clk, prof, req)
	if err != nil {
		return nil, nil, err
	}

	// generate template from the issuer's data
	template := i.generateTemplate()

	ekus := []x509.ExtKeyUsage{
		x509.ExtKeyUsageServerAuth,
		x509.ExtKeyUsageClientAuth,
	}
	if prof.omitClientAuth {
		ekus = []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		}
	}
	template.ExtKeyUsage = ekus

	// populate template from the issuance request
	template.NotBefore, template.NotAfter = req.NotBefore, req.NotAfter
	template.SerialNumber = big.NewInt(0).SetBytes(req.Serial)
	if req.CommonName != "" && !prof.omitCommonName {
		template.Subject.CommonName = req.CommonName
	}
	template.DNSNames = req.DNSNames
	template.IPAddresses = req.IPAddresses

	switch req.PublicKey.PublicKey.(type) {
	case *rsa.PublicKey:
		if prof.omitKeyEncipherment {
			template.KeyUsage = x509.KeyUsageDigitalSignature
		} else {
			template.KeyUsage = x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment
		}
	case *ecdsa.PublicKey:
		template.KeyUsage = x509.KeyUsageDigitalSignature
	}

	if !prof.omitSKID {
		template.SubjectKeyId = req.SubjectKeyId
	}

	if req.IncludeCTPoison {
		template.ExtraExtensions = append(template.ExtraExtensions, ctPoisonExt)
	} else if len(req.sctList) > 0 {
		if len(req.precertDER) == 0 {
			return nil, nil, errors.New("inconsistent request contains sctList but no precertDER")
		}
		sctListExt, err := generateSCTListExt(req.sctList)
		if err != nil {
			return nil, nil, err
		}
		template.ExtraExtensions = append(template.ExtraExtensions, sctListExt)
	} else {
		return nil, nil, errors.New("invalid request contains neither sctList nor precertDER")
	}

	// If explicit CRL sharding is enabled, pick a shard based on the serial number
	// modulus the number of shards. This gives us random distribution that is
	// nonetheless consistent between precert and cert.
	if prof.includeCRLDistributionPoints {
		if i.crlShards <= 0 {
			return nil, nil, errors.New("IncludeCRLDistributionPoints was set but CRLShards was not set")
		}
		shardZeroBased := big.NewInt(0).Mod(template.SerialNumber, big.NewInt(int64(i.crlShards)))
		shard := int(shardZeroBased.Int64()) + 1
		url := i.crlURL(shard)
		template.CRLDistributionPoints = []string{url}
	}

	// check that the tbsCertificate is properly formed by signing it
	// with a throwaway key and then linting it using zlint
	lintCertBytes, err := i.Linter.Check(template, req.PublicKey.PublicKey, prof.lints)
	if err != nil {
		return nil, nil, fmt.Errorf("tbsCertificate linting failed: %w", err)
	}

	if len(req.precertDER) > 0 {
		err = precert.Correspond(req.precertDER, lintCertBytes)
		if err != nil {
			return nil, nil, fmt.Errorf("precert does not correspond to linted final cert: %w", err)
		}
	}

	token := &issuanceToken{sync.Mutex{}, template, req.PublicKey, i}
	return lintCertBytes, token, nil
}

// Issue performs a real issuance using an issuanceToken resulting from a
// previous call to Prepare(). Call this at most once per token. Calls after
// the first will receive an error.
func (i *Issuer) Issue(token *issuanceToken) ([]byte, error) {
	if token == nil {
		return nil, errors.New("nil issuanceToken")
	}
	token.mu.Lock()
	defer token.mu.Unlock()
	if token.template == nil {
		return nil, errors.New("issuance token already redeemed")
	}
	template := token.template
	token.template = nil

	if token.issuer != i {
		return nil, errors.New("tried to redeem issuance token with the wrong issuer")
	}

	return x509.CreateCertificate(rand.Reader, template, i.Cert.Certificate, token.pubKey.PublicKey, i.Signer)
}

// containsCTPoison returns true if the provided set of extensions includes
// an entry whose OID and value both match the expected values for the CT
// Poison extension.
func containsCTPoison(extensions []pkix.Extension) bool {
	for _, ext := range extensions {
		if ext.Id.Equal(ctPoisonExt.Id) && bytes.Equal(ext.Value, asn1.NullBytes) {
			return true
		}
	}
	return false
}

// RequestFromPrecert constructs a final certificate IssuanceRequest matching
// the provided precertificate. It returns an error if the precertificate doesn't
// contain the CT poison extension.
func RequestFromPrecert(precert *x509.Certificate, scts []ct.SignedCertificateTimestamp) (*IssuanceRequest, error) {
	if !containsCTPoison(precert.Extensions) {
		return nil, errors.New("provided certificate doesn't contain the CT poison extension")
	}
	return &IssuanceRequest{
		PublicKey:    MarshalablePublicKey{precert.PublicKey},
		SubjectKeyId: precert.SubjectKeyId,
		Serial:       precert.SerialNumber.Bytes(),
		NotBefore:    precert.NotBefore,
		NotAfter:     precert.NotAfter,
		CommonName:   precert.Subject.CommonName,
		DNSNames:     precert.DNSNames,
		IPAddresses:  precert.IPAddresses,
		sctList:      scts,
		precertDER:   precert.Raw,
	}, nil
}
