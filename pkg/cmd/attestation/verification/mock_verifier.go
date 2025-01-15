package verification

import (
	"fmt"
	"testing"

	"github.com/cli/cli/v2/pkg/cmd/attestation/api"
	"github.com/cli/cli/v2/pkg/cmd/attestation/test/data"
	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/fulcio/certificate"

	in_toto "github.com/in-toto/attestation/go/v1"
	"github.com/sigstore/sigstore-go/pkg/verify"
)

type MockSigstoreVerifier struct {
	t           *testing.T
	mockResults []*AttestationProcessingResult
}

func (v *MockSigstoreVerifier) Verify([]*api.Attestation, verify.PolicyBuilder) ([]*AttestationProcessingResult, error) {
	if v.mockResults != nil {
		return v.mockResults, nil
	}

	statement := &in_toto.Statement{}
	statement.PredicateType = SLSAPredicateV1

	result := AttestationProcessingResult{
		Attestation: &api.Attestation{
			Bundle: data.SigstoreBundle(v.t),
		},
		VerificationResult: &verify.VerificationResult{
			Statement: statement,
			Signature: &verify.SignatureVerificationResult{
				Certificate: &certificate.Summary{
					Extensions: certificate.Extensions{
						BuildSignerURI:           "https://github.com/github/example/.github/workflows/release.yml@refs/heads/main",
						SourceRepositoryOwnerURI: "https://github.com/sigstore",
						SourceRepositoryURI:      "https://github.com/sigstore/sigstore-js",
						Issuer:                   "https://token.actions.githubusercontent.com",
					},
				},
			},
		},
	}

	results := []*AttestationProcessingResult{&result}

	return results, nil
}

func NewMockSigstoreVerifier(t *testing.T) *MockSigstoreVerifier {
	result := BuildSigstoreJsMockResult(t)
	results := []*AttestationProcessingResult{&result}

	return &MockSigstoreVerifier{t, results}
}

func NewMockSigstoreVerifierWithMockResults(t *testing.T, mockResults []*AttestationProcessingResult) *MockSigstoreVerifier {
	return &MockSigstoreVerifier{t, mockResults}
}

type FailSigstoreVerifier struct{}

func (v *FailSigstoreVerifier) Verify([]*api.Attestation, verify.PolicyBuilder) ([]*AttestationProcessingResult, error) {
	return nil, fmt.Errorf("failed to verify attestations")
}

func BuildMockResult(b *bundle.Bundle, buildSignerURI, sourceRepoOwnerURI, sourceRepoURI, issuer string) AttestationProcessingResult {
	statement := &in_toto.Statement{}
	statement.PredicateType = SLSAPredicateV1

	return AttestationProcessingResult{
		Attestation: &api.Attestation{
			Bundle: b,
		},
		VerificationResult: &verify.VerificationResult{
			Statement: statement,
			Signature: &verify.SignatureVerificationResult{
				Certificate: &certificate.Summary{
					Extensions: certificate.Extensions{
						BuildSignerURI:           buildSignerURI,
						SourceRepositoryOwnerURI: sourceRepoOwnerURI,
						SourceRepositoryURI:      sourceRepoURI,
						Issuer:                   issuer,
					},
				},
			},
		},
	}
}

func BuildSigstoreJsMockResult(t *testing.T) AttestationProcessingResult {
	bundle := data.SigstoreBundle(t)
	buildSignerURI := "https://github.com/github/example/.github/workflows/release.yml@refs/heads/main"
	sourceRepoOwnerURI := "https://github.com/sigstore"
	sourceRepoURI := "https://github.com/sigstore/sigstore-js"
	issuer := "https://token.actions.githubusercontent.com"
	return BuildMockResult(bundle, buildSignerURI, sourceRepoOwnerURI, sourceRepoURI, issuer)
}
