package rfc

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/zmap/zcrypto/x509"
	"github.com/zmap/zlint/v3/lint"
	"github.com/zmap/zlint/v3/util"
)

type certViaPKILint struct {
	PKILintAddr    string        `toml:"pkilint_addr" comment:"The address where a pkilint REST API can be reached."`
	PKILintTimeout time.Duration `toml:"pkilint_timeout" comment:"How long, in nanoseconds, to wait before giving up."`
	IgnoreLints    []string      `toml:"ignore_lints" comment:"The unique Validator:Code IDs of lint findings which should be ignored."`
}

func init() {
	lint.RegisterCertificateLint(&lint.CertificateLint{
		LintMetadata: lint.LintMetadata{
			Name:          "e_pkilint_lint_cabf_serverauth_cert",
			Description:   "Runs pkilint's suite of cabf serverauth certificate lints",
			Citation:      "https://github.com/digicert/pkilint",
			Source:        lint.Community,
			EffectiveDate: util.CABEffectiveDate,
		},
		Lint: NewCertValidityNotRound,
	})
}

func NewCertValidityNotRound() lint.CertificateLintInterface {
	return &certViaPKILint{}
}

func (l *certViaPKILint) Configure() interface{} {
	return l
}

func (l *certViaPKILint) CheckApplies(c *x509.Certificate) bool {
	// This lint applies to all certificates issued by Boulder, as long as it has
	// been configured with an address to reach out to. If not, skip it.
	return l.PKILintAddr != ""
}

type PKILintResponse struct {
	Results []struct {
		Validator           string `json:"validator"`
		NodePath            string `json:"node_path"`
		FindingDescriptions []struct {
			Severity string `json:"severity"`
			Code     string `json:"code"`
			Message  string `json:"message,omitempty"`
		} `json:"finding_descriptions"`
	} `json:"results"`
	Linter struct {
		Name string `json:"name"`
	} `json:"linter"`
}

func (l *certViaPKILint) Execute(c *x509.Certificate) *lint.LintResult {
	timeout := l.PKILintTimeout
	if timeout == 0 {
		timeout = 100 * time.Millisecond
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	reqJSON, err := json.Marshal(struct {
		B64 string `json:"b64"`
	}{
		B64: base64.StdEncoding.EncodeToString(c.Raw),
	})
	if err != nil {
		return &lint.LintResult{
			Status:  lint.Error,
			Details: fmt.Sprintf("marshalling pkilint request: %s", err),
		}
	}

	url := fmt.Sprintf("%s/certificate/cabf-serverauth", l.PKILintAddr)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqJSON))
	if err != nil {
		return &lint.LintResult{
			Status:  lint.Error,
			Details: fmt.Sprintf("creating pkilint request: %s", err),
		}
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return &lint.LintResult{
			Status:  lint.Error,
			Details: fmt.Sprintf("making POST request to pkilint API: %s", err),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &lint.LintResult{
			Status:  lint.Error,
			Details: fmt.Sprintf("got status %d (%s) from pkilint API", resp.StatusCode, resp.Status),
		}
	}

	res, err := io.ReadAll(resp.Body)
	if err != nil {
		return &lint.LintResult{
			Status:  lint.Error,
			Details: fmt.Sprintf("reading response from pkilint API: %s", err),
		}
	}

	var jsonResult PKILintResponse
	err = json.Unmarshal(res, &jsonResult)
	if err != nil {
		return &lint.LintResult{
			Status:  lint.Error,
			Details: fmt.Sprintf("parsing response from pkilint API: %s", err),
		}
	}

	var findings []string
	for _, validator := range jsonResult.Results {
		for _, finding := range validator.FindingDescriptions {
			id := fmt.Sprintf("%s:%s", validator.Validator, finding.Code)
			if slices.Contains(l.IgnoreLints, id) {
				continue
			}
			desc := fmt.Sprintf("%s from %s at %s", finding.Severity, id, validator.NodePath)
			if finding.Message != "" {
				desc = fmt.Sprintf("%s: %s", desc, finding.Message)
			}
			findings = append(findings, desc)
		}
	}

	if len(findings) != 0 {
		// Group the findings by severity, for human readers.
		slices.Sort(findings)
		return &lint.LintResult{
			Status:  lint.Error,
			Details: fmt.Sprintf("got %d lint findings from pkilint API: %s", len(findings), strings.Join(findings, "; ")),
		}
	}

	return &lint.LintResult{Status: lint.Pass}
}
