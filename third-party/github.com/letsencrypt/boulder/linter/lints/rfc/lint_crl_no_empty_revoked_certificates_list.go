package rfc

import (
	"github.com/zmap/zcrypto/x509"
	"github.com/zmap/zlint/v3/lint"
	"github.com/zmap/zlint/v3/util"
)

type crlNoEmptyRevokedCertsList struct{}

/************************************************
RFC 5280: 5.1.2.6
When there are no revoked certificates, the revoked certificates list MUST be
absent.
************************************************/

func init() {
	lint.RegisterRevocationListLint(&lint.RevocationListLint{
		LintMetadata: lint.LintMetadata{
			Name:          "e_crl_no_empty_revoked_certificates_list",
			Description:   "When there are no revoked certificates, the revoked certificates list MUST be absent.",
			Citation:      "RFC 5280: 5.1.2.6",
			Source:        lint.RFC5280,
			EffectiveDate: util.RFC5280Date,
		},
		Lint: NewCrlNoEmptyRevokedCertsList,
	})
}

func NewCrlNoEmptyRevokedCertsList() lint.RevocationListLintInterface {
	return &crlNoEmptyRevokedCertsList{}
}

func (l *crlNoEmptyRevokedCertsList) CheckApplies(c *x509.RevocationList) bool {
	return true
}

func (l *crlNoEmptyRevokedCertsList) Execute(c *x509.RevocationList) *lint.LintResult {
	if c.RevokedCertificates != nil && len(c.RevokedCertificates) == 0 {
		return &lint.LintResult{
			Status:  lint.Error,
			Details: "If the revokedCertificates list is empty, it must not be present",
		}
	}
	return &lint.LintResult{Status: lint.Pass}
}
