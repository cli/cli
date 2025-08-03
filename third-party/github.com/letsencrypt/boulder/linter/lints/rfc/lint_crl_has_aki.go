package rfc

import (
	"github.com/zmap/zcrypto/x509"
	"github.com/zmap/zlint/v3/lint"
	"github.com/zmap/zlint/v3/util"
	"golang.org/x/crypto/cryptobyte"
	cryptobyte_asn1 "golang.org/x/crypto/cryptobyte/asn1"
)

type crlHasAKI struct{}

/************************************************
RFC 5280: 5.2.1
Conforming CRL issuers MUST use the key identifier method, and MUST include this
extension in all CRLs issued.
************************************************/

func init() {
	lint.RegisterRevocationListLint(&lint.RevocationListLint{
		LintMetadata: lint.LintMetadata{
			Name:          "e_crl_has_aki",
			Description:   "Conforming",
			Citation:      "RFC 5280: 5.2.1",
			Source:        lint.RFC5280,
			EffectiveDate: util.RFC5280Date,
		},
		Lint: NewCrlHasAKI,
	})
}

func NewCrlHasAKI() lint.RevocationListLintInterface {
	return &crlHasAKI{}
}

func (l *crlHasAKI) CheckApplies(c *x509.RevocationList) bool {
	return true
}

func (l *crlHasAKI) Execute(c *x509.RevocationList) *lint.LintResult {
	if len(c.AuthorityKeyId) == 0 {
		return &lint.LintResult{
			Status:  lint.Error,
			Details: "CRLs MUST include the authority key identifier extension",
		}
	}
	aki := cryptobyte.String(c.AuthorityKeyId)
	var akiBody cryptobyte.String
	if !aki.ReadASN1(&akiBody, cryptobyte_asn1.SEQUENCE) {
		return &lint.LintResult{
			Status:  lint.Error,
			Details: "CRL has a malformed authority key identifier extension",
		}
	}
	if !akiBody.PeekASN1Tag(cryptobyte_asn1.Tag(0).ContextSpecific()) {
		return &lint.LintResult{
			Status:  lint.Error,
			Details: "CRLs MUST use the key identifier method in the authority key identifier extension",
		}
	}
	return &lint.LintResult{Status: lint.Pass}
}
