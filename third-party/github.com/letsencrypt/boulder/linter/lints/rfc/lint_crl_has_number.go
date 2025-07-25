package rfc

import (
	"github.com/zmap/zcrypto/encoding/asn1"
	"github.com/zmap/zcrypto/x509"
	"github.com/zmap/zlint/v3/lint"
	"github.com/zmap/zlint/v3/util"

	"github.com/letsencrypt/boulder/linter/lints"
)

type crlHasNumber struct{}

/************************************************
RFC 5280: 5.2.3
CRL issuers conforming to this profile MUST include this extension in all CRLs
and MUST mark this extension as non-critical. Conforming CRL issuers MUST NOT
use CRLNumber values longer than 20 octets.
************************************************/

func init() {
	lint.RegisterRevocationListLint(&lint.RevocationListLint{
		LintMetadata: lint.LintMetadata{
			Name:          "e_crl_has_number",
			Description:   "CRLs must have a well-formed CRL Number extension",
			Citation:      "RFC 5280: 5.2.3",
			Source:        lint.RFC5280,
			EffectiveDate: util.RFC5280Date,
		},
		Lint: NewCrlHasNumber,
	})
}

func NewCrlHasNumber() lint.RevocationListLintInterface {
	return &crlHasNumber{}
}

func (l *crlHasNumber) CheckApplies(c *x509.RevocationList) bool {
	return true
}

func (l *crlHasNumber) Execute(c *x509.RevocationList) *lint.LintResult {
	if c.Number == nil {
		return &lint.LintResult{
			Status:  lint.Error,
			Details: "CRLs MUST include the CRL number extension",
		}
	}

	crlNumberOID := asn1.ObjectIdentifier{2, 5, 29, 20} // id-ce-cRLNumber
	ext := lints.GetExtWithOID(c.Extensions, crlNumberOID)
	if ext != nil && ext.Critical {
		return &lint.LintResult{
			Status:  lint.Error,
			Details: "CRL Number MUST NOT be marked critical",
		}
	}

	numBytes := c.Number.Bytes()
	if len(numBytes) > 20 || (len(numBytes) == 20 && numBytes[0]&0x80 != 0) {
		return &lint.LintResult{
			Status:  lint.Error,
			Details: "CRL Number MUST NOT be longer than 20 octets",
		}
	}
	return &lint.LintResult{Status: lint.Pass}
}
