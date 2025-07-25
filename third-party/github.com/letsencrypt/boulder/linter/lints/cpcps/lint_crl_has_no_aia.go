package cpcps

import (
	"github.com/zmap/zcrypto/encoding/asn1"
	"github.com/zmap/zcrypto/x509"
	"github.com/zmap/zlint/v3/lint"

	"github.com/letsencrypt/boulder/linter/lints"
)

type crlHasNoAIA struct{}

/************************************************
RFC 5280: 5.2.7

The requirements around the Authority Information Access extension are extensive.
Therefore we do not include one.
Conforming CRL issuers MUST include the nextUpdate field in all CRLs.
************************************************/

func init() {
	lint.RegisterRevocationListLint(&lint.RevocationListLint{
		LintMetadata: lint.LintMetadata{
			Name:          "e_crl_has_no_aia",
			Description:   "Let's Encrypt does not include the CRL AIA extension",
			Citation:      "",
			Source:        lints.LetsEncryptCPS,
			EffectiveDate: lints.CPSV33Date,
		},
		Lint: NewCrlHasNoAIA,
	})
}

func NewCrlHasNoAIA() lint.RevocationListLintInterface {
	return &crlHasNoAIA{}
}

func (l *crlHasNoAIA) CheckApplies(c *x509.RevocationList) bool {
	return true
}

func (l *crlHasNoAIA) Execute(c *x509.RevocationList) *lint.LintResult {
	aiaOID := asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 1, 1} // id-pe-authorityInfoAccess
	if lints.GetExtWithOID(c.Extensions, aiaOID) != nil {
		return &lint.LintResult{
			Status:  lint.Notice,
			Details: "CRL has an Authority Information Access url",
		}
	}
	return &lint.LintResult{Status: lint.Pass}
}
