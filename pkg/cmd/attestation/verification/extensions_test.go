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
		err := VerifyCertExtensions(results, "", "owner", "owner/repo")
		require.NoError(t, err)
	})

	t.Run("VerifyCertExtensions with owner and repo, but wrong tenant", func(t *testing.T) {
		err := VerifyCertExtensions(results, "foo", "owner", "owner/repo")
		require.ErrorContains(t, err, "expected SourceRepositoryOwnerURI to be https://foo.ghe.com/owner, got https://github.com/owner")
	})

	t.Run("VerifyCertExtensions with owner", func(t *testing.T) {
		err := VerifyCertExtensions(results, "", "owner", "")
		require.NoError(t, err)
	})

	t.Run("VerifyCertExtensions with wrong owner", func(t *testing.T) {
		err := VerifyCertExtensions(results, "", "wrong", "")
		require.ErrorContains(t, err, "expected SourceRepositoryOwnerURI to be https://github.com/wrong, got https://github.com/owner")
	})

	t.Run("VerifyCertExtensions with wrong repo", func(t *testing.T) {
		err := VerifyCertExtensions(results, "", "owner", "wrong")
		require.ErrorContains(t, err, "expected SourceRepositoryURI to be https://github.com/wrong, got https://github.com/owner/repo")
	})
}

func TestVerifyTenancyCertExtensions(t *testing.T) {
	results := []*AttestationProcessingResult{
		{
			VerificationResult: &verify.VerificationResult{
				Signature: &verify.SignatureVerificationResult{
					Certificate: &certificate.Summary{
						Extensions: certificate.Extensions{
							SourceRepositoryOwnerURI: "https://foo.ghe.com/owner",
							SourceRepositoryURI:      "https://foo.ghe.com/owner/repo",
						},
					},
				},
			},
		},
	}

	t.Run("VerifyCertExtensions with owner and repo", func(t *testing.T) {
		err := VerifyCertExtensions(results, "foo", "owner", "owner/repo")
		require.NoError(t, err)
	})

	t.Run("VerifyCertExtensions with owner and repo, no tenant", func(t *testing.T) {
		err := VerifyCertExtensions(results, "", "owner", "owner/repo")
		require.ErrorContains(t, err, "expected SourceRepositoryOwnerURI to be https://github.com/owner, got https://foo.ghe.com/owner")
	})

	t.Run("VerifyCertExtensions with owner and repo, wrong tenant", func(t *testing.T) {
		err := VerifyCertExtensions(results, "bar", "owner", "owner/repo")
		require.ErrorContains(t, err, "expected SourceRepositoryOwnerURI to be https://bar.ghe.com/owner, got https://foo.ghe.com/owner")
	})

	t.Run("VerifyCertExtensions with owner", func(t *testing.T) {
		err := VerifyCertExtensions(results, "foo", "owner", "")
		require.NoError(t, err)
	})

	t.Run("VerifyCertExtensions with wrong owner", func(t *testing.T) {
		err := VerifyCertExtensions(results, "foo", "wrong", "")
		require.ErrorContains(t, err, "expected SourceRepositoryOwnerURI to be https://foo.ghe.com/wrong, got https://foo.ghe.com/owner")
	})

	t.Run("VerifyCertExtensions with wrong repo", func(t *testing.T) {
		err := VerifyCertExtensions(results, "foo", "owner", "wrong")
		require.ErrorContains(t, err, "expected SourceRepositoryURI to be https://foo.ghe.com/wrong, got https://foo.ghe.com/owner/repo")
	})
}
