package linter

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"fmt"
	"strings"

	zlintx509 "github.com/zmap/zcrypto/x509"
	"github.com/zmap/zlint/v3"
	"github.com/zmap/zlint/v3/lint"

	"github.com/letsencrypt/boulder/core"

	_ "github.com/letsencrypt/boulder/linter/lints/cabf_br"
	_ "github.com/letsencrypt/boulder/linter/lints/chrome"
	_ "github.com/letsencrypt/boulder/linter/lints/cpcps"
	_ "github.com/letsencrypt/boulder/linter/lints/rfc"
)

var ErrLinting = fmt.Errorf("failed lint(s)")

// Check accomplishes the entire process of linting: it generates a throwaway
// signing key, uses that to create a linting cert, and runs a default set of
// lints (everything except for the ETSI and EV lints) against it. If the
// subjectPubKey and realSigner indicate that this is a self-signed cert, the
// cert will have its pubkey replaced to also be self-signed. This is the
// primary public interface of this package, but it can be inefficient; creating
// a new signer and a new lint registry are expensive operations which
// performance-sensitive clients may want to cache via linter.New().
func Check(tbs *x509.Certificate, subjectPubKey crypto.PublicKey, realIssuer *x509.Certificate, realSigner crypto.Signer, skipLints []string) ([]byte, error) {
	linter, err := New(realIssuer, realSigner)
	if err != nil {
		return nil, err
	}

	reg, err := NewRegistry(skipLints)
	if err != nil {
		return nil, err
	}

	lintCertBytes, err := linter.Check(tbs, subjectPubKey, reg)
	if err != nil {
		return nil, err
	}

	return lintCertBytes, nil
}

// CheckCRL is like Check, but for CRLs.
func CheckCRL(tbs *x509.RevocationList, realIssuer *x509.Certificate, realSigner crypto.Signer, skipLints []string) error {
	linter, err := New(realIssuer, realSigner)
	if err != nil {
		return err
	}

	reg, err := NewRegistry(skipLints)
	if err != nil {
		return err
	}

	return linter.CheckCRL(tbs, reg)
}

// Linter is capable of linting a to-be-signed (TBS) certificate. It does so by
// signing that certificate with a throwaway private key and a fake issuer whose
// public key matches the throwaway private key, and then running the resulting
// certificate through a registry of zlint lints.
type Linter struct {
	issuer     *x509.Certificate
	signer     crypto.Signer
	realPubKey crypto.PublicKey
}

// New constructs a Linter. It uses the provided real certificate and signer
// (private key) to generate a matching fake keypair and issuer cert that will
// be used to sign the lint certificate. It uses the provided list of lint names
// to skip to filter the zlint global registry to only those lints which should
// be run.
func New(realIssuer *x509.Certificate, realSigner crypto.Signer) (*Linter, error) {
	lintSigner, err := makeSigner(realSigner)
	if err != nil {
		return nil, err
	}
	lintIssuer, err := makeIssuer(realIssuer, lintSigner)
	if err != nil {
		return nil, err
	}
	return &Linter{lintIssuer, lintSigner, realSigner.Public()}, nil
}

// Check signs the given TBS certificate using the Linter's fake issuer cert and
// private key, then runs the resulting certificate through all lints in reg.
// If the subjectPubKey is identical to the public key of the real signer
// used to create this linter, then the throwaway cert will have its pubkey
// replaced with the linter's pubkey so that it appears self-signed. It returns
// an error if any lint fails. On success it also returns the DER bytes of the
// linting certificate.
func (l Linter) Check(tbs *x509.Certificate, subjectPubKey crypto.PublicKey, reg lint.Registry) ([]byte, error) {
	lintPubKey := subjectPubKey
	selfSigned, err := core.PublicKeysEqual(subjectPubKey, l.realPubKey)
	if err != nil {
		return nil, err
	}
	if selfSigned {
		lintPubKey = l.signer.Public()
	}

	lintCertBytes, cert, err := makeLintCert(tbs, lintPubKey, l.issuer, l.signer)
	if err != nil {
		return nil, err
	}

	lintRes := zlint.LintCertificateEx(cert, reg)
	err = ProcessResultSet(lintRes)
	if err != nil {
		return nil, err
	}

	return lintCertBytes, nil
}

// CheckCRL signs the given RevocationList template using the Linter's fake
// issuer cert and private key, then runs the resulting CRL through all CRL
// lints in the registry. It returns an error if any check fails.
func (l Linter) CheckCRL(tbs *x509.RevocationList, reg lint.Registry) error {
	crl, err := makeLintCRL(tbs, l.issuer, l.signer)
	if err != nil {
		return err
	}
	lintRes := zlint.LintRevocationListEx(crl, reg)
	return ProcessResultSet(lintRes)
}

func makeSigner(realSigner crypto.Signer) (crypto.Signer, error) {
	var lintSigner crypto.Signer
	var err error
	switch k := realSigner.Public().(type) {
	case *rsa.PublicKey:
		lintSigner, err = rsa.GenerateKey(rand.Reader, k.Size()*8)
		if err != nil {
			return nil, fmt.Errorf("failed to create RSA lint signer: %w", err)
		}
	case *ecdsa.PublicKey:
		lintSigner, err = ecdsa.GenerateKey(k.Curve, rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("failed to create ECDSA lint signer: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported lint signer type: %T", k)
	}
	return lintSigner, nil
}

func makeIssuer(realIssuer *x509.Certificate, lintSigner crypto.Signer) (*x509.Certificate, error) {
	lintIssuerTBS := &x509.Certificate{
		// This is nearly the full list of attributes that
		// x509.CreateCertificate() says it carries over from the template.
		// Constructing this TBS certificate in this way ensures that the
		// resulting lint issuer is as identical to the real issuer as we can
		// get, without sharing a public key.
		//
		// We do not copy the SignatureAlgorithm field while constructing the
		// lintIssuer because the lintIssuer is self-signed. Depending on the
		// realIssuer, which could be either an intermediate or cross-signed
		// intermediate, the SignatureAlgorithm of that certificate may differ
		// from the root certificate that had signed it.
		AuthorityKeyId:              realIssuer.AuthorityKeyId,
		BasicConstraintsValid:       realIssuer.BasicConstraintsValid,
		CRLDistributionPoints:       realIssuer.CRLDistributionPoints,
		DNSNames:                    realIssuer.DNSNames,
		EmailAddresses:              realIssuer.EmailAddresses,
		ExcludedDNSDomains:          realIssuer.ExcludedDNSDomains,
		ExcludedEmailAddresses:      realIssuer.ExcludedEmailAddresses,
		ExcludedIPRanges:            realIssuer.ExcludedIPRanges,
		ExcludedURIDomains:          realIssuer.ExcludedURIDomains,
		ExtKeyUsage:                 realIssuer.ExtKeyUsage,
		ExtraExtensions:             realIssuer.ExtraExtensions,
		IPAddresses:                 realIssuer.IPAddresses,
		IsCA:                        realIssuer.IsCA,
		IssuingCertificateURL:       realIssuer.IssuingCertificateURL,
		KeyUsage:                    realIssuer.KeyUsage,
		MaxPathLen:                  realIssuer.MaxPathLen,
		MaxPathLenZero:              realIssuer.MaxPathLenZero,
		NotAfter:                    realIssuer.NotAfter,
		NotBefore:                   realIssuer.NotBefore,
		OCSPServer:                  realIssuer.OCSPServer,
		PermittedDNSDomains:         realIssuer.PermittedDNSDomains,
		PermittedDNSDomainsCritical: realIssuer.PermittedDNSDomainsCritical,
		PermittedEmailAddresses:     realIssuer.PermittedEmailAddresses,
		PermittedIPRanges:           realIssuer.PermittedIPRanges,
		PermittedURIDomains:         realIssuer.PermittedURIDomains,
		Policies:                    realIssuer.Policies,
		SerialNumber:                realIssuer.SerialNumber,
		Subject:                     realIssuer.Subject,
		SubjectKeyId:                realIssuer.SubjectKeyId,
		URIs:                        realIssuer.URIs,
		UnknownExtKeyUsage:          realIssuer.UnknownExtKeyUsage,
	}
	lintIssuerBytes, err := x509.CreateCertificate(rand.Reader, lintIssuerTBS, lintIssuerTBS, lintSigner.Public(), lintSigner)
	if err != nil {
		return nil, fmt.Errorf("failed to create lint issuer: %w", err)
	}
	lintIssuer, err := x509.ParseCertificate(lintIssuerBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse lint issuer: %w", err)
	}
	return lintIssuer, nil
}

// NewRegistry returns a zlint Registry with irrelevant (ETSI, EV) lints
// excluded. This registry also includes all custom lints defined in Boulder.
func NewRegistry(skipLints []string) (lint.Registry, error) {
	reg, err := lint.GlobalRegistry().Filter(lint.FilterOptions{
		ExcludeNames: skipLints,
		ExcludeSources: []lint.LintSource{
			// Excluded because Boulder does not issue EV certs.
			lint.CABFEVGuidelines,
			// Excluded because Boulder does not use the
			// ETSI EN 319 412-5 qcStatements extension.
			lint.EtsiEsi,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create lint registry: %w", err)
	}
	return reg, nil
}

func makeLintCert(tbs *x509.Certificate, subjectPubKey crypto.PublicKey, issuer *x509.Certificate, signer crypto.Signer) ([]byte, *zlintx509.Certificate, error) {
	lintCertBytes, err := x509.CreateCertificate(rand.Reader, tbs, issuer, subjectPubKey, signer)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create lint certificate: %w", err)
	}
	lintCert, err := zlintx509.ParseCertificate(lintCertBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse lint certificate: %w", err)
	}
	// RFC 5280, Sections 4.1.2.6 and 8
	//
	// When the subject of the certificate is a CA, the subject
	// field MUST be encoded in the same way as it is encoded in the
	// issuer field (Section 4.1.2.4) in all certificates issued by
	// the subject CA.
	if !bytes.Equal(issuer.RawSubject, lintCert.RawIssuer) {
		return nil, nil, fmt.Errorf("mismatch between lint issuer RawSubject and lintCert.RawIssuer DER bytes: \"%x\" != \"%x\"", issuer.RawSubject, lintCert.RawIssuer)
	}

	return lintCertBytes, lintCert, nil
}

func ProcessResultSet(lintRes *zlint.ResultSet) error {
	if lintRes.NoticesPresent || lintRes.WarningsPresent || lintRes.ErrorsPresent || lintRes.FatalsPresent {
		var failedLints []string
		for lintName, result := range lintRes.Results {
			if result.Status > lint.Pass {
				failedLints = append(failedLints, fmt.Sprintf("%s (%s)", lintName, result.Details))
			}
		}
		return fmt.Errorf("%w: %s", ErrLinting, strings.Join(failedLints, ", "))
	}
	return nil
}

func makeLintCRL(tbs *x509.RevocationList, issuer *x509.Certificate, signer crypto.Signer) (*zlintx509.RevocationList, error) {
	lintCRLBytes, err := x509.CreateRevocationList(rand.Reader, tbs, issuer, signer)
	if err != nil {
		return nil, err
	}
	lintCRL, err := zlintx509.ParseRevocationList(lintCRLBytes)
	if err != nil {
		return nil, err
	}
	return lintCRL, nil
}
