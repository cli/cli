package cabfbr

import (
	"github.com/zmap/zcrypto/encoding/asn1"
	"github.com/zmap/zcrypto/x509"
	"github.com/zmap/zlint/v3/lint"
	"github.com/zmap/zlint/v3/util"
)

type crlCriticalReasonCodes struct{}

/************************************************
Baseline Requirements: 7.2.2.1:
If present, [the reasonCode] extension MUST NOT be marked critical.
************************************************/

func init() {
	lint.RegisterRevocationListLint(&lint.RevocationListLint{
		LintMetadata: lint.LintMetadata{
			Name:          "e_crl_no_critical_reason_codes",
			Description:   "CRL entry reasonCode extension MUST NOT be marked critical",
			Citation:      "BRs: 7.2.2.1",
			Source:        lint.CABFBaselineRequirements,
			EffectiveDate: util.CABFBRs_1_8_0_Date,
		},
		Lint: NewCrlCriticalReasonCodes,
	})
}

func NewCrlCriticalReasonCodes() lint.RevocationListLintInterface {
	return &crlCriticalReasonCodes{}
}

func (l *crlCriticalReasonCodes) CheckApplies(c *x509.RevocationList) bool {
	return true
}

func (l *crlCriticalReasonCodes) Execute(c *x509.RevocationList) *lint.LintResult {
	reasonCodeOID := asn1.ObjectIdentifier{2, 5, 29, 21} // id-ce-reasonCode
	for _, rc := range c.RevokedCertificates {
		for _, ext := range rc.Extensions {
			if ext.Id.Equal(reasonCodeOID) && ext.Critical {
				return &lint.LintResult{
					Status:  lint.Error,
					Details: "CRL entry reasonCode extension MUST NOT be marked critical",
				}
			}
		}
	}
	return &lint.LintResult{Status: lint.Pass}
}
