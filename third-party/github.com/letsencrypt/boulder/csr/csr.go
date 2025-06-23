package csr

import (
	"context"
	"crypto"
	"crypto/x509"
	"errors"
	"strings"

	"github.com/letsencrypt/boulder/core"
	berrors "github.com/letsencrypt/boulder/errors"
	"github.com/letsencrypt/boulder/goodkey"
)

// maxCNLength is the maximum length allowed for the common name as specified in RFC 5280
const maxCNLength = 64

// This map is used to decide which CSR signing algorithms we consider
// strong enough to use. Significantly the missing algorithms are:
// * No algorithms using MD2, MD5, or SHA-1
// * No DSA algorithms
var goodSignatureAlgorithms = map[x509.SignatureAlgorithm]bool{
	x509.SHA256WithRSA:   true,
	x509.SHA384WithRSA:   true,
	x509.SHA512WithRSA:   true,
	x509.ECDSAWithSHA256: true,
	x509.ECDSAWithSHA384: true,
	x509.ECDSAWithSHA512: true,
}

var (
	invalidPubKey       = berrors.BadCSRError("invalid public key in CSR")
	unsupportedSigAlg   = berrors.BadCSRError("signature algorithm not supported")
	invalidSig          = berrors.BadCSRError("invalid signature on CSR")
	invalidEmailPresent = berrors.BadCSRError("CSR contains one or more email address fields")
	invalidIPPresent    = berrors.BadCSRError("CSR contains one or more IP address fields")
	invalidNoDNS        = berrors.BadCSRError("at least one DNS name is required")
)

// VerifyCSR checks the validity of a x509.CertificateRequest. Before doing checks it normalizes
// the CSR which lowers the case of DNS names and subject CN, and hoist a DNS name into the CN
// if it is empty.
func VerifyCSR(ctx context.Context, csr *x509.CertificateRequest, maxNames int, keyPolicy *goodkey.KeyPolicy, pa core.PolicyAuthority) error {
	key, ok := csr.PublicKey.(crypto.PublicKey)
	if !ok {
		return invalidPubKey
	}
	err := keyPolicy.GoodKey(ctx, key)
	if err != nil {
		if errors.Is(err, goodkey.ErrBadKey) {
			return berrors.BadCSRError("invalid public key in CSR: %s", err)
		}
		return berrors.InternalServerError("error checking key validity: %s", err)
	}
	if !goodSignatureAlgorithms[csr.SignatureAlgorithm] {
		return unsupportedSigAlg
	}

	err = csr.CheckSignature()
	if err != nil {
		return invalidSig
	}
	if len(csr.EmailAddresses) > 0 {
		return invalidEmailPresent
	}
	if len(csr.IPAddresses) > 0 {
		return invalidIPPresent
	}

	names := NamesFromCSR(csr)

	if len(names.SANs) == 0 && names.CN == "" {
		return invalidNoDNS
	}
	if len(names.CN) > maxCNLength {
		return berrors.BadCSRError("CN was longer than %d bytes", maxCNLength)
	}
	if len(names.SANs) > maxNames {
		return berrors.BadCSRError("CSR contains more than %d DNS names", maxNames)
	}

	err = pa.WillingToIssue(names.SANs)
	if err != nil {
		return err
	}
	return nil
}

type names struct {
	SANs []string
	CN   string
}

// NamesFromCSR deduplicates and lower-cases the Subject Common Name and Subject
// Alternative Names from the CSR. If the CSR contains a CN, then it preserves
// it and guarantees that the SANs also include it. If the CSR does not contain
// a CN, then it also attempts to promote a SAN to the CN (if any is short
// enough to fit).
func NamesFromCSR(csr *x509.CertificateRequest) names {
	// Produce a new "sans" slice with the same memory address as csr.DNSNames
	// but force a new allocation if an append happens so that we don't
	// accidentally mutate the underlying csr.DNSNames array.
	sans := csr.DNSNames[0:len(csr.DNSNames):len(csr.DNSNames)]
	if csr.Subject.CommonName != "" {
		sans = append(sans, csr.Subject.CommonName)
	}

	if csr.Subject.CommonName != "" {
		return names{SANs: core.UniqueLowerNames(sans), CN: strings.ToLower(csr.Subject.CommonName)}
	}

	// If there's no CN already, but we want to set one, promote the first SAN
	// which is shorter than the maximum acceptable CN length (if any).
	for _, name := range sans {
		if len(name) <= maxCNLength {
			return names{SANs: core.UniqueLowerNames(sans), CN: strings.ToLower(name)}
		}
	}

	return names{SANs: core.UniqueLowerNames(sans)}
}
