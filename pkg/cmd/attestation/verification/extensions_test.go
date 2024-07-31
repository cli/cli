package verification

import (
	"testing"

	"github.com/sigstore/sigstore-go/pkg/fulcio/certificate"
	"github.com/sigstore/sigstore-go/pkg/verify"
	"github.com/stretchr/testify/require"
)

func TestVerifyCertExtensions(t *testing.T) {
	results := []*AttestationProcessingResult{
		{
			VerificationResult: &verify.VerificationResult{
				Signature: &verify.SignatureVerificationResult{
					Certificate: &certificate.Summary{
						Extensions: certificate.Extensions{
							SourceRepositoryOwnerURI: "https://github.com/owner",
							SourceRepositoryURI:      "https://github.com/owner/repo",
						},
					},
				},
			},
		},
	}

	t.Run("VerifyCertExtensions with owner and repo", func(t *testing.T) {
		err := VerifyCertExtensions(results, "owner", "owner/repo")
		require.NoError(t, err)
	})

	t.Run("VerifyCertExtensions with owner", func(t *testing.T) {
		err := VerifyCertExtensions(results, "owner", "")
		require.NoError(t, err)
	})

	t.Run("VerifyCertExtensions with wrong owner", func(t *testing.T) {
		err := VerifyCertExtensions(results, "wrong", "")
		require.ErrorContains(t, err, "expected SourceRepositoryOwnerURI to be https://github.com/wrong, got https://github.com/owner")
	})

	t.Run("VerifyCertExtensions with wrong repo", func(t *testing.T) {
		err := VerifyCertExtensions(results, "owner", "wrong")
		require.ErrorContains(t, err, "expected SourceRepositoryURI to be https://github.com/wrong, got https://github.com/owner/repo")
	})
}
