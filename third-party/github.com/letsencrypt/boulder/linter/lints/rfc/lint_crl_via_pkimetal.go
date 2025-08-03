package rfc

import (
	"github.com/zmap/zcrypto/x509"
	"github.com/zmap/zlint/v3/lint"
	"github.com/zmap/zlint/v3/util"
)

type crlViaPKIMetal struct {
	PKIMetalConfig
}

func init() {
	lint.RegisterRevocationListLint(&lint.RevocationListLint{
		LintMetadata: lint.LintMetadata{
			Name:          "e_pkimetal_lint_cabf_serverauth_crl",
			Description:   "Runs pkimetal's suite of cabf serverauth CRL lints",
			Citation:      "https://github.com/pkimetal/pkimetal",
			Source:        lint.Community,
			EffectiveDate: util.CABEffectiveDate,
		},
		Lint: NewCrlViaPKIMetal,
	})
}

func NewCrlViaPKIMetal() lint.RevocationListLintInterface {
	return &crlViaPKIMetal{}
}

func (l *crlViaPKIMetal) Configure() any {
	return l
}

func (l *crlViaPKIMetal) CheckApplies(c *x509.RevocationList) bool {
	// This lint applies to all CRLs issued by Boulder, as long as it has
	// been configured with an address to reach out to. If not, skip it.
	return l.Addr != ""
}

func (l *crlViaPKIMetal) Execute(c *x509.RevocationList) *lint.LintResult {
	res, err := l.execute("lintcrl", c.Raw)
	if err != nil {
		return &lint.LintResult{
			Status:  lint.Error,
			Details: err.Error(),
		}
	}

	return res
}
