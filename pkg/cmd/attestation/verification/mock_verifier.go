package verification

import (
	"fmt"
	"testing"

	"github.com/cli/cli/v2/pkg/cmd/attestation/api"
	"github.com/cli/cli/v2/pkg/cmd/attestation/test/data"
	"github.com/sigstore/sigstore-go/pkg/fulcio/certificate"

	in_toto "github.com/in-toto/attestation/go/v1"
	"github.com/sigstore/sigstore-go/pkg/verify"
)

const SLSAPredicateType = "https://slsa.dev/provenance/v1"

type MockSigstoreVerifier struct {
	t *testing.T
}

func (v *MockSigstoreVerifier) Verify(attestations []*api.Attestation, policy verify.PolicyBuilder) *SigstoreResults {
	statement := &in_toto.Statement{}
	statement.PredicateType = SLSAPredicateType

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

	return &SigstoreResults{
		VerifyResults: results,
	}
}

func NewMockSigstoreVerifier(t *testing.T) *MockSigstoreVerifier {
	return &MockSigstoreVerifier{t}
}

type FailSigstoreVerifier struct{}

func (v *FailSigstoreVerifier) Verify(attestations []*api.Attestation, policy verify.PolicyBuilder) *SigstoreResults {
	return &SigstoreResults{
		Error: fmt.Errorf("failed to verify attestations"),
	}
}
