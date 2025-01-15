package verification

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact"

	"github.com/sigstore/sigstore-go/pkg/fulcio/certificate"
	"github.com/sigstore/sigstore-go/pkg/verify"
)

// represents the GitHub hosted runner in the certificate RunnerEnvironment extension
const GitHubRunner = "github-hosted"

// BuildDigestPolicyOption builds a verify.ArtifactPolicyOption
// from the given artifact digest and digest algorithm
func BuildDigestPolicyOption(a artifact.DigestedArtifact) (verify.ArtifactPolicyOption, error) {
	// sigstore-go expects the artifact digest to be decoded from hex
	decoded, err := hex.DecodeString(a.Digest())
	if err != nil {
		return nil, err
	}
	return verify.WithArtifactDigest(a.Algorithm(), decoded), nil
}

type EnforcementCriteria struct {
	Certificate   certificate.Summary
	PredicateType string
	SANRegex      string
	SAN           string
}

func (c EnforcementCriteria) Valid() error {
	if c.Certificate.Issuer == "" {
		return fmt.Errorf("Issuer must be set")
	}
	if c.Certificate.RunnerEnvironment != "" && c.Certificate.RunnerEnvironment != GitHubRunner {
		return fmt.Errorf("RunnerEnvironment must be set to either \"\" or %s", GitHubRunner)
	}
	if c.Certificate.SourceRepositoryOwnerURI == "" {
		return fmt.Errorf("SourceRepositoryOwnerURI must be set")
	}
	if c.PredicateType == "" {
		return fmt.Errorf("PredicateType must be set")
	}
	if c.SANRegex == "" && c.SAN == "" {
		return fmt.Errorf("SANRegex or SAN must be set")
	}
	return nil
}

func (c EnforcementCriteria) BuildPolicyInformation() string {
	policyAttr := make([][]string, 0, 6)

	policyAttr = appendStr(policyAttr, "- OIDC Issuer must match", c.Certificate.Issuer)
	if c.Certificate.RunnerEnvironment == GitHubRunner {
		policyAttr = appendStr(policyAttr, "- Action workflow Runner Environment must match ", GitHubRunner)
	}

	policyAttr = appendStr(policyAttr, "- Source Repository Owner URI must match", c.Certificate.SourceRepositoryOwnerURI)

	if c.Certificate.SourceRepositoryURI != "" {
		policyAttr = appendStr(policyAttr, "- Source Repository URI must match", c.Certificate.SourceRepositoryURI)
	}

	policyAttr = appendStr(policyAttr, "- Predicate type must match", c.PredicateType)

	if c.SAN != "" {
		policyAttr = appendStr(policyAttr, "- Subject Alternative Name must match", c.SAN)
	} else if c.SANRegex != "" {
		policyAttr = appendStr(policyAttr, "- Subject Alternative Name must match regex", c.SANRegex)
	}

	maxColLen := 0
	for _, attr := range policyAttr {
		if len(attr[0]) > maxColLen {
			maxColLen = len(attr[0])
		}
	}

	policyInfo := ""
	for _, attr := range policyAttr {
		dots := strings.Repeat(".", maxColLen-len(attr[0]))
		policyInfo += fmt.Sprintf("%s:%s %s\n", attr[0], dots, attr[1])
	}

	return policyInfo
}

func appendStr(arr [][]string, a, b string) [][]string {
	return append(arr, []string{a, b})
}
