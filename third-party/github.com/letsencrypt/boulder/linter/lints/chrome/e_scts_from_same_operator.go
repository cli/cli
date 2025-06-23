package chrome

import (
	"time"

	"github.com/zmap/zcrypto/x509"
	"github.com/zmap/zcrypto/x509/ct"
	"github.com/zmap/zlint/v3/lint"
	"github.com/zmap/zlint/v3/util"

	"github.com/letsencrypt/boulder/ctpolicy/loglist"
	"github.com/letsencrypt/boulder/linter/lints"
)

type sctsFromSameOperator struct {
	logList loglist.List
}

func init() {
	lint.RegisterCertificateLint(&lint.CertificateLint{
		LintMetadata: lint.LintMetadata{
			Name:          "e_scts_from_same_operator",
			Description:   "Let's Encrypt Subscriber Certificates have two SCTs from logs run by different operators",
			Citation:      "Chrome CT Policy",
			Source:        lints.ChromeCTPolicy,
			EffectiveDate: time.Date(2022, time.April, 15, 0, 0, 0, 0, time.UTC),
		},
		Lint: NewSCTsFromSameOperator,
	})
}

func NewSCTsFromSameOperator() lint.CertificateLintInterface {
	return &sctsFromSameOperator{logList: loglist.GetLintList()}
}

func (l *sctsFromSameOperator) CheckApplies(c *x509.Certificate) bool {
	return util.IsSubscriberCert(c) && !util.IsExtInCert(c, util.CtPoisonOID)
}

func (l *sctsFromSameOperator) Execute(c *x509.Certificate) *lint.LintResult {
	if len(l.logList) == 0 {
		return &lint.LintResult{
			Status:  lint.NE,
			Details: "Failed to load log list, unable to check Certificate SCTs.",
		}
	}

	if len(c.SignedCertificateTimestampList) < 2 {
		return &lint.LintResult{
			Status:  lint.Error,
			Details: "Certificate had too few embedded SCTs; browser policy requires 2.",
		}
	}

	logIDs := make(map[ct.SHA256Hash]struct{})
	for _, sct := range c.SignedCertificateTimestampList {
		logIDs[sct.LogID] = struct{}{}
	}

	if len(logIDs) < 2 {
		return &lint.LintResult{
			Status:  lint.Error,
			Details: "Certificate SCTs from too few distinct logs; browser policy requires 2.",
		}
	}

	operatorNames := make(map[string]struct{})
	for logID := range logIDs {
		operator, err := l.logList.OperatorForLogID(logID.Base64String())
		if err != nil {
			// This certificate *may* have more than 2 SCTs, so missing one now isn't
			// a problem.
			continue
		}
		operatorNames[operator] = struct{}{}
	}

	if len(operatorNames) < 2 {
		return &lint.LintResult{
			Status:  lint.Error,
			Details: "Certificate SCTs from too few distinct log operators; browser policy requires 2.",
		}
	}

	return &lint.LintResult{
		Status: lint.Pass,
	}
}
