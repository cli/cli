package cpcps

import (
	"github.com/zmap/zcrypto/encoding/asn1"
	"github.com/zmap/zcrypto/x509"
	"github.com/zmap/zlint/v3/lint"

	"github.com/letsencrypt/boulder/linter/lints"
)

type crlHasNoCertIssuers struct{}

/************************************************
RFC 5280: 5.3.3

Section 5.3.3 defines the Certificate Issuer entry extension. The presence of
this extension means that the CRL is an "indirect CRL", including certificates
which were issued by a different issuer than the one issuing the CRL itself.
We do not issue indirect CRLs, so our CRL entries should not have this extension.
************************************************/

func init() {
	lint.RegisterRevocationListLint(&lint.RevocationListLint{
		LintMetadata: lint.LintMetadata{
			Name:          "e_crl_has_no_cert_issuers",
			Description:   "Let's Encrypt does not issue indirect CRLs",
			Citation:      "",
			Source:        lints.LetsEncryptCPS,
			EffectiveDate: lints.CPSV33Date,
		},
		Lint: NewCrlHasNoCertIssuers,
	})
}

func NewCrlHasNoCertIssuers() lint.RevocationListLintInterface {
	return &crlHasNoCertIssuers{}
}

func (l *crlHasNoCertIssuers) CheckApplies(c *x509.RevocationList) bool {
	return true
}

func (l *crlHasNoCertIssuers) Execute(c *x509.RevocationList) *lint.LintResult {
	certIssuerOID := asn1.ObjectIdentifier{2, 5, 29, 29} // id-ce-certificateIssuer
	for _, entry := range c.RevokedCertificates {
		if lints.GetExtWithOID(entry.Extensions, certIssuerOID) != nil {
			return &lint.LintResult{
				Status:  lint.Notice,
				Details: "CRL has an entry with a Certificate Issuer extension",
			}
		}
	}
	return &lint.LintResult{Status: lint.Pass}
}
