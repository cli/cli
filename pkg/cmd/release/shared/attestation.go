package shared

import (
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/pkg/cmd/attestation/api"
	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact"
	att_io "github.com/cli/cli/v2/pkg/cmd/attestation/io"
	"github.com/cli/cli/v2/pkg/cmd/attestation/test/data"
	"github.com/cli/cli/v2/pkg/cmd/attestation/verification"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/sigstore/sigstore-go/pkg/fulcio/certificate"
	"github.com/sigstore/sigstore-go/pkg/verify"

	v1 "github.com/in-toto/attestation/go/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

const ReleasePredicateType = "https://in-toto.io/attestation/release/v0.1"

type Verifier interface {
	// VerifyAttestation verifies the attestation for a given artifact
	VerifyAttestation(art *artifact.DigestedArtifact, att *api.Attestation) (*verification.AttestationProcessingResult, error)
}

type AttestationVerifier struct {
	AttClient  api.Client
	HttpClient *http.Client
	IO         *iostreams.IOStreams
}

func (v *AttestationVerifier) VerifyAttestation(art *artifact.DigestedArtifact, att *api.Attestation) (*verification.AttestationProcessingResult, error) {
	td, err := v.AttClient.GetTrustDomain()
	if err != nil {
		return nil, err
	}

	verifier, err := verification.NewLiveSigstoreVerifier(verification.SigstoreConfig{
		HttpClient:   v.HttpClient,
		Logger:       att_io.NewHandler(v.IO),
		NoPublicGood: true,
		TrustDomain:  td,
	})
	if err != nil {
		return nil, err
	}

	policy := buildVerificationPolicy(*art)
	sigstoreVerified, err := verifier.Verify([]*api.Attestation{att}, policy)
	if err != nil {
		return nil, err
	}

	return sigstoreVerified[0], nil
}

func FilterAttestationsByTag(attestations []*api.Attestation, tagName string) ([]*api.Attestation, error) {
	var filtered []*api.Attestation
	for _, att := range attestations {
		statement := att.Bundle.Bundle.GetDsseEnvelope().Payload
		var statementData v1.Statement
		err := protojson.Unmarshal([]byte(statement), &statementData)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal statement: %w", err)
		}
		tagValue := statementData.Predicate.GetFields()["tag"].GetStringValue()

		if tagValue == tagName {
			filtered = append(filtered, att)
		}
	}
	return filtered, nil
}

func FilterAttestationsByFileDigest(attestations []*api.Attestation, fileDigest string) ([]*api.Attestation, error) {
	var filtered []*api.Attestation
	for _, att := range attestations {
		statement := att.Bundle.Bundle.GetDsseEnvelope().Payload
		var statementData v1.Statement
		err := protojson.Unmarshal([]byte(statement), &statementData)

		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal statement: %w", err)
		}
		subjects := statementData.Subject
		for _, subject := range subjects {
			digestMap := subject.GetDigest()
			alg := "sha256"

			digest := digestMap[alg]
			if digest == fileDigest {
				filtered = append(filtered, att)
			}
		}

	}
	return filtered, nil
}

// buildVerificationPolicy constructs a verification policy for GitHub releases
func buildVerificationPolicy(a artifact.DigestedArtifact) verify.PolicyBuilder {
	// SAN must match the GitHub releases domain. No issuer extension (match anything)
	sanMatcher, _ := verify.NewSANMatcher("", "^https://.*\\.releases\\.github\\.com$")
	issuerMatcher, _ := verify.NewIssuerMatcher("", ".*")
	certId, _ := verify.NewCertificateIdentity(sanMatcher, issuerMatcher, certificate.Extensions{})

	artifactDigestPolicyOption, _ := verification.BuildDigestPolicyOption(a)
	return verify.NewPolicy(artifactDigestPolicyOption, verify.WithCertificateIdentity(certId))
}

type MockVerifier struct {
	mockResult *verification.AttestationProcessingResult
}

func NewMockVerifier(mockResult *verification.AttestationProcessingResult) *MockVerifier {
	return &MockVerifier{mockResult: mockResult}
}

func (v *MockVerifier) VerifyAttestation(art *artifact.DigestedArtifact, att *api.Attestation) (*verification.AttestationProcessingResult, error) {
	return &verification.AttestationProcessingResult{
		Attestation: &api.Attestation{
			Bundle:    data.GitHubReleaseBundle(nil),
			BundleURL: "https://example.com",
		},
		VerificationResult: nil,
	}, nil
}
