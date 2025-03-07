package verification

import (
	"testing"

	"github.com/sigstore/sigstore-go/pkg/fulcio/certificate"
	"github.com/sigstore/sigstore-go/pkg/verify"
	"github.com/stretchr/testify/require"
)

func createSampleResult() *AttestationProcessingResult {
	return &AttestationProcessingResult{
		VerificationResult: &verify.VerificationResult{
			Signature: &verify.SignatureVerificationResult{
				Certificate: &certificate.Summary{
					Extensions: certificate.Extensions{
						SourceRepositoryOwnerURI: "https://github.com/owner",
						SourceRepositoryURI:      "https://github.com/owner/repo",
						Issuer:                   "https://token.actions.githubusercontent.com",
					},
				},
			},
		},
	}
}

func TestVerifyCertExtensions(t *testing.T) {
	results := []*AttestationProcessingResult{createSampleResult()}

	certSummary := certificate.Summary{}
	certSummary.SourceRepositoryOwnerURI = "https://github.com/owner"
	certSummary.SourceRepositoryURI = "https://github.com/owner/repo"
	certSummary.Issuer = GitHubOIDCIssuer

	c := EnforcementCriteria{
		Certificate: certSummary,
	}

	t.Run("passes with one result", func(t *testing.T) {
		verified, err := VerifyCertExtensions(results, c)
		require.NoError(t, err)
		require.Len(t, verified, 1)
	})

	t.Run("passes with 1/2 valid results", func(t *testing.T) {
		twoResults := []*AttestationProcessingResult{createSampleResult(), createSampleResult()}
		require.Len(t, twoResults, 2)
		twoResults[1].VerificationResult.Signature.Certificate.Extensions.SourceRepositoryOwnerURI = "https://github.com/wrong"

		verified, err := VerifyCertExtensions(twoResults, c)
		require.NoError(t, err)
		require.Len(t, verified, 1)
	})

	t.Run("fails when all results fail verification", func(t *testing.T) {
		twoResults := []*AttestationProcessingResult{createSampleResult(), createSampleResult()}
		require.Len(t, twoResults, 2)
		twoResults[0].VerificationResult.Signature.Certificate.Extensions.SourceRepositoryOwnerURI = "https://github.com/wrong"
		twoResults[1].VerificationResult.Signature.Certificate.Extensions.SourceRepositoryOwnerURI = "https://github.com/wrong"

		verified, err := VerifyCertExtensions(twoResults, c)
		require.Error(t, err)
		require.Nil(t, verified)
	})

	t.Run("with wrong SourceRepositoryOwnerURI", func(t *testing.T) {
		expectedCriteria := c
		expectedCriteria.Certificate.SourceRepositoryOwnerURI = "https://github.com/wrong"
		verified, err := VerifyCertExtensions(results, expectedCriteria)
		require.ErrorContains(t, err, "expected SourceRepositoryOwnerURI to be https://github.com/wrong, got https://github.com/owner")
		require.Nil(t, verified)
	})

	t.Run("with wrong SourceRepositoryURI", func(t *testing.T) {
		expectedCriteria := c
		expectedCriteria.Certificate.SourceRepositoryURI = "https://github.com/foo/wrong"
		verified, err := VerifyCertExtensions(results, expectedCriteria)
		require.ErrorContains(t, err, "expected SourceRepositoryURI to be https://github.com/foo/wrong, got https://github.com/owner/repo")
		require.Nil(t, verified)
	})

	t.Run("with wrong OIDCIssuer", func(t *testing.T) {
		expectedCriteria := c
		expectedCriteria.Certificate.Issuer = "wrong"
		verified, err := VerifyCertExtensions(results, expectedCriteria)
		require.ErrorContains(t, err, "expected Issuer to be wrong, got https://token.actions.githubusercontent.com")
		require.Nil(t, verified)
	})

	t.Run("with partial OIDCIssuer match", func(t *testing.T) {
		expectedResults := results
		expectedResults[0].VerificationResult.Signature.Certificate.Extensions.Issuer = "https://token.actions.githubusercontent.com/foo-bar"
		verified, err := VerifyCertExtensions(expectedResults, c)
		require.ErrorContains(t, err, "expected Issuer to be https://token.actions.githubusercontent.com, got https://token.actions.githubusercontent.com/foo-bar -- if you have a custom OIDC issuer")
		require.Nil(t, verified)
	})
}
