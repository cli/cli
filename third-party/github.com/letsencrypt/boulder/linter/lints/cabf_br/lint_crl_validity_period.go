package cabfbr

import (
	"fmt"
	"time"

	"github.com/letsencrypt/boulder/linter/lints"
	"github.com/zmap/zcrypto/encoding/asn1"
	"github.com/zmap/zcrypto/x509"
	"github.com/zmap/zlint/v3/lint"
	"github.com/zmap/zlint/v3/util"
	"golang.org/x/crypto/cryptobyte"

	cryptobyte_asn1 "golang.org/x/crypto/cryptobyte/asn1"
)

type crlValidityPeriod struct{}

/************************************************
Baseline Requirements, Section 4.9.7:
* For the status of Subscriber Certificates [...] the value of the nextUpdate
  field MUST NOT be more than ten days beyond the value of the thisUpdate field.
* For the status of Subordinate CA Certificates [...]. The value of the
  nextUpdate field MUST NOT be more than twelve months beyond the value of the
  thisUpdatefield.
************************************************/

func init() {
	lint.RegisterRevocationListLint(&lint.RevocationListLint{
		LintMetadata: lint.LintMetadata{
			Name:          "e_crl_validity_period",
			Description:   "Let's Encrypt CRLs must have an acceptable validity period",
			Citation:      "BRs: 4.9.7",
			Source:        lint.CABFBaselineRequirements,
			EffectiveDate: util.CABFBRs_1_2_1_Date,
		},
		Lint: NewCrlValidityPeriod,
	})
}

func NewCrlValidityPeriod() lint.RevocationListLintInterface {
	return &crlValidityPeriod{}
}

func (l *crlValidityPeriod) CheckApplies(c *x509.RevocationList) bool {
	return true
}

func (l *crlValidityPeriod) Execute(c *x509.RevocationList) *lint.LintResult {
	/*
	   Let's Encrypt issues two kinds of CRLs:

	    1) CRLs containing subscriber certificates, created by crl-updater.
	       These assert the distributionPoint and onlyContainsUserCerts
	       boolean.
	    2) CRLs containing issuer CRLs, created by the ceremony tool. These
	       assert the onlyContainsCACerts boolean.

	   We use the presence of these booleans to determine which BR-mandated
	   lifetime to enforce.
	*/

	// The only way to determine which type of CRL we're dealing with. The
	// issuingDistributionPoint must be parsed and the internal fields
	// inspected.
	idpOID := asn1.ObjectIdentifier{2, 5, 29, 28} // id-ce-issuingDistributionPoint
	idpe := lints.GetExtWithOID(c.Extensions, idpOID)
	if idpe == nil {
		return &lint.LintResult{
			Status:  lint.Warn,
			Details: "CRL missing IssuingDistributionPoint",
		}
	}

	// Step inside the outer issuingDistributionPoint sequence to get access to
	// its constituent fields.
	idpv := cryptobyte.String(idpe.Value)
	if !idpv.ReadASN1(&idpv, cryptobyte_asn1.SEQUENCE) {
		return &lint.LintResult{
			Status:  lint.Warn,
			Details: "Failed to read IssuingDistributionPoint distributionPoint",
		}
	}

	// Throw distributionPoint away.
	distributionPointTag := cryptobyte_asn1.Tag(0).ContextSpecific().Constructed()
	_ = idpv.SkipOptionalASN1(distributionPointTag)

	// Parse IssuingDistributionPoint OPTIONAL BOOLEANS to eventually perform
	// sanity checks.
	idp := lints.NewIssuingDistributionPoint()
	onlyContainsUserCertsTag := cryptobyte_asn1.Tag(1).ContextSpecific()
	if !lints.ReadOptionalASN1BooleanWithTag(&idpv, &idp.OnlyContainsUserCerts, onlyContainsUserCertsTag, false) {
		return &lint.LintResult{
			Status:  lint.Warn,
			Details: "Failed to read IssuingDistributionPoint onlyContainsUserCerts",
		}
	}

	onlyContainsCACertsTag := cryptobyte_asn1.Tag(2).ContextSpecific()
	if !lints.ReadOptionalASN1BooleanWithTag(&idpv, &idp.OnlyContainsCACerts, onlyContainsCACertsTag, false) {
		return &lint.LintResult{
			Status:  lint.Warn,
			Details: "Failed to read IssuingDistributionPoint onlyContainsCACerts",
		}
	}

	// Basic sanity check so that later on we can determine what type of CRL we
	// issued based on the presence of one of these fields. If both fields exist
	// then 1) it's a problem and 2) the real validity period is unknown.
	if idp.OnlyContainsUserCerts && idp.OnlyContainsCACerts {
		return &lint.LintResult{
			Status:  lint.Error,
			Details: "IssuingDistributionPoint should not have both onlyContainsUserCerts: TRUE and onlyContainsCACerts: TRUE",
		}
	}

	// Default to subscriber cert CRL.
	var BRValidity = 10 * 24 * time.Hour
	var validityString = "10 days"
	if idp.OnlyContainsCACerts {
		BRValidity = 365 * lints.BRDay
		validityString = "365 days"
	}

	parsedValidity := c.NextUpdate.Sub(c.ThisUpdate)
	if parsedValidity <= 0 {
		return &lint.LintResult{
			Status:  lint.Error,
			Details: "CRL has NextUpdate at or before ThisUpdate",
		}
	}

	if parsedValidity > BRValidity {
		return &lint.LintResult{
			Status:  lint.Error,
			Details: fmt.Sprintf("CRL has validity period greater than %s", validityString),
		}
	}
	return &lint.LintResult{Status: lint.Pass}
}
