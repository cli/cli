package cpcps

import (
	"time"

	"github.com/zmap/zcrypto/x509"
	"github.com/zmap/zlint/v3/lint"

	"github.com/letsencrypt/boulder/linter/lints"
)

type certValidityNotRound struct{}

func init() {
	lint.RegisterCertificateLint(&lint.CertificateLint{
		LintMetadata: lint.LintMetadata{
			Name:          "w_validity_period_has_extra_second",
			Description:   "Let's Encrypt Certificates have Validity Periods that are a round number of seconds",
			Citation:      "CPS: 7.1",
			Source:        lints.LetsEncryptCPS,
			EffectiveDate: lints.CPSV33Date,
		},
		Lint: NewCertValidityNotRound,
	})
}

func NewCertValidityNotRound() lint.CertificateLintInterface {
	return &certValidityNotRound{}
}

func (l *certValidityNotRound) CheckApplies(c *x509.Certificate) bool {
	return true
}

func (l *certValidityNotRound) Execute(c *x509.Certificate) *lint.LintResult {
	// RFC 5280 4.1.2.5: "The validity period for a certificate is the period
	// of time from notBefore through notAfter, inclusive."
	certValidity := c.NotAfter.Add(time.Second).Sub(c.NotBefore)

	if certValidity%60 == 0 {
		return &lint.LintResult{Status: lint.Pass}
	}

	return &lint.LintResult{Status: lint.Error}
}
