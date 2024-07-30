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

	err := VerifyCertExtensions(results, "owner", "owner/repo")
	require.NoError(t, err)

	err = VerifyCertExtensions(results, "", "owner/repo")
	require.NoError(t, err)

	err = VerifyCertExtensions(results, "owner", "")
	require.NoError(t, err)

	err = VerifyCertExtensions(results, "wrong", "")
	require.ErrorContains(t, err, "expected SourceRepositoryOwnerURI to be https://github.com/wrong, got https://github.com/owner")

	err = VerifyCertExtensions(results, "", "wrong")
	require.ErrorContains(t, err, "expected SourceRepositoryURI to be https://github.com/wrong, got https://github.com/owner/repo")
}
