package cabfbr

import (
	"github.com/zmap/zcrypto/x509"
	"github.com/zmap/zlint/v3/lint"

	"github.com/letsencrypt/boulder/linter/lints"
)

type crlAcceptableReasonCodes struct{}

/************************************************
Baseline Requirements: 7.2.2.1:
The CRLReason indicated MUST NOT be unspecified (0).
The CRLReason MUST NOT be certificateHold (6).

When the CRLReason code is not one of the following, then the reasonCode extension MUST NOT be provided:
- keyCompromise (RFC 5280 CRLReason #1);
- privilegeWithdrawn (RFC 5280 CRLReason #9);
- cessationOfOperation (RFC 5280 CRLReason #5);
- affiliationChanged (RFC 5280 CRLReason #3); or
- superseded (RFC 5280 CRLReason #4).
************************************************/

func init() {
	lint.RegisterRevocationListLint(&lint.RevocationListLint{
		LintMetadata: lint.LintMetadata{
			Name:        "e_crl_acceptable_reason_codes",
			Description: "CRL entry Reason Codes must be 1, 3, 4, 5, or 9",
			Citation:    "BRs: 7.2.2.1",
			Source:      lint.CABFBaselineRequirements,
			// We use the Mozilla Root Store Policy v2.8.1 effective date here
			// because, although this lint enforces requirements from the BRs, those
			// same requirements were in the MRSP first.
			EffectiveDate: lints.MozillaPolicy281Date,
		},
		Lint: NewCrlAcceptableReasonCodes,
	})
}

func NewCrlAcceptableReasonCodes() lint.RevocationListLintInterface {
	return &crlAcceptableReasonCodes{}
}

func (l *crlAcceptableReasonCodes) CheckApplies(c *x509.RevocationList) bool {
	return true
}

func (l *crlAcceptableReasonCodes) Execute(c *x509.RevocationList) *lint.LintResult {
	for _, rc := range c.RevokedCertificates {
		if rc.ReasonCode == nil {
			continue
		}
		switch *rc.ReasonCode {
		case 1: // keyCompromise
		case 3: // affiliationChanged
		case 4: // superseded
		case 5: // cessationOfOperation
		case 9: // privilegeWithdrawn
			continue
		default:
			return &lint.LintResult{
				Status:  lint.Error,
				Details: "CRLs MUST NOT include reasonCodes other than 1, 3, 4, 5, and 9",
			}
		}
	}
	return &lint.LintResult{Status: lint.Pass}
}
