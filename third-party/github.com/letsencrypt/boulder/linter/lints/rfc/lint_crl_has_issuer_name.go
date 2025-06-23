package rfc

import (
	"github.com/zmap/zcrypto/x509"
	"github.com/zmap/zlint/v3/lint"
	"github.com/zmap/zlint/v3/util"
)

type crlHasIssuerName struct{}

/************************************************
RFC 5280: 5.1.2.3
The issuer field MUST contain a non-empty X.500 distinguished name (DN).

This lint does not enforce that the issuer field complies with the rest of
the encoding rules of a certificate issuer name, because it (perhaps wrongly)
assumes that those were checked when the issuer was itself issued, and on all
certificates issued by this CRL issuer.
************************************************/

func init() {
	lint.RegisterRevocationListLint(&lint.RevocationListLint{
		LintMetadata: lint.LintMetadata{
			Name:          "e_crl_has_issuer_name",
			Description:   "The CRL Issuer field MUST contain a non-empty X.500 distinguished name",
			Citation:      "RFC 5280: 5.1.2.3",
			Source:        lint.RFC5280,
			EffectiveDate: util.RFC5280Date,
		},
		Lint: NewCrlHasIssuerName,
	})
}

func NewCrlHasIssuerName() lint.RevocationListLintInterface {
	return &crlHasIssuerName{}
}

func (l *crlHasIssuerName) CheckApplies(c *x509.RevocationList) bool {
	return true
}

func (l *crlHasIssuerName) Execute(c *x509.RevocationList) *lint.LintResult {
	if len(c.Issuer.Names) == 0 {
		return &lint.LintResult{
			Status:  lint.Error,
			Details: "The CRL Issuer field MUST contain a non-empty X.500 distinguished name",
		}
	}
	return &lint.LintResult{Status: lint.Pass}
}
