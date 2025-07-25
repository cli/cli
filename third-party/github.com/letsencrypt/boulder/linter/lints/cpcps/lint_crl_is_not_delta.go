package cpcps

import (
	"github.com/zmap/zcrypto/encoding/asn1"
	"github.com/zmap/zcrypto/x509"
	"github.com/zmap/zlint/v3/lint"

	"github.com/letsencrypt/boulder/linter/lints"
)

type crlIsNotDelta struct{}

/************************************************
RFC 5280: 5.2.4

Section 5.2.4 defines a Delta CRL, and all the requirements that come with it.
These requirements are complex and do not serve our purpose, so we ensure that
we never issue a CRL which could be construed as a Delta CRL.

RFC 5280: 5.2.6

Similarly, Section 5.2.6 defines the Freshest CRL extension, which is only
applicable in the case that the CRL is a Delta CRL.
************************************************/

func init() {
	lint.RegisterRevocationListLint(&lint.RevocationListLint{
		LintMetadata: lint.LintMetadata{
			Name:          "e_crl_is_not_delta",
			Description:   "Let's Encrypt does not issue delta CRLs",
			Citation:      "",
			Source:        lints.LetsEncryptCPS,
			EffectiveDate: lints.CPSV33Date,
		},
		Lint: NewCrlIsNotDelta,
	})
}

func NewCrlIsNotDelta() lint.RevocationListLintInterface {
	return &crlIsNotDelta{}
}

func (l *crlIsNotDelta) CheckApplies(c *x509.RevocationList) bool {
	return true
}

func (l *crlIsNotDelta) Execute(c *x509.RevocationList) *lint.LintResult {
	deltaCRLIndicatorOID := asn1.ObjectIdentifier{2, 5, 29, 27} // id-ce-deltaCRLIndicator
	if lints.GetExtWithOID(c.Extensions, deltaCRLIndicatorOID) != nil {
		return &lint.LintResult{
			Status:  lint.Notice,
			Details: "CRL is a Delta CRL",
		}
	}

	freshestCRLOID := asn1.ObjectIdentifier{2, 5, 29, 46} // id-ce-freshestCRL
	if lints.GetExtWithOID(c.Extensions, freshestCRLOID) != nil {
		return &lint.LintResult{
			Status:  lint.Notice,
			Details: "CRL has a Freshest CRL url",
		}
	}

	return &lint.LintResult{Status: lint.Pass}
}
