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
							Issuer:                   "https://token.actions.githubusercontent.com",
						},
					},
				},
			},
		},
	}

	t.Run("VerifyCertExtensions with owner and repo", func(t *testing.T) {
		err := VerifyCertExtensions(results, "", "owner", "owner/repo", GitHubOIDCIssuer)
		require.NoError(t, err)
	})

	t.Run("VerifyCertExtensions with owner and repo, but wrong tenant", func(t *testing.T) {
		err := VerifyCertExtensions(results, "foo", "owner", "owner/repo", GitHubOIDCIssuer)
		require.ErrorContains(t, err, "expected SourceRepositoryOwnerURI to be https://foo.ghe.com/owner, got https://github.com/owner")
	})

	t.Run("VerifyCertExtensions with owner", func(t *testing.T) {
		err := VerifyCertExtensions(results, "", "owner", "", GitHubOIDCIssuer)
		require.NoError(t, err)
	})

	t.Run("VerifyCertExtensions with wrong owner", func(t *testing.T) {
		err := VerifyCertExtensions(results, "", "wrong", "", GitHubOIDCIssuer)
		require.ErrorContains(t, err, "expected SourceRepositoryOwnerURI to be https://github.com/wrong, got https://github.com/owner")
	})

	t.Run("VerifyCertExtensions with wrong repo", func(t *testing.T) {
		err := VerifyCertExtensions(results, "", "owner", "wrong", GitHubOIDCIssuer)
		require.ErrorContains(t, err, "expected SourceRepositoryURI to be https://github.com/wrong, got https://github.com/owner/repo")
	})

	t.Run("VerifyCertExtensions with wrong issuer", func(t *testing.T) {
		err := VerifyCertExtensions(results, "", "owner", "", "wrong")
		require.ErrorContains(t, err, "expected Issuer to be wrong, got https://token.actions.githubusercontent.com")
	})
}

func TestVerifyCertExtensionsCustomizedIssuer(t *testing.T) {
	results := []*AttestationProcessingResult{
		{
			VerificationResult: &verify.VerificationResult{
				Signature: &verify.SignatureVerificationResult{
					Certificate: &certificate.Summary{
						Extensions: certificate.Extensions{
							SourceRepositoryOwnerURI: "https://github.com/owner",
							SourceRepositoryURI:      "https://github.com/owner/repo",
							Issuer:                   "https://token.actions.githubusercontent.com/foo-bar",
						},
					},
				},
			},
		},
	}

	t.Run("VerifyCertExtensions with exact issuer match", func(t *testing.T) {
		err := VerifyCertExtensions(results, "", "owner", "owner/repo", "https://token.actions.githubusercontent.com/foo-bar")
		require.NoError(t, err)
	})

	t.Run("VerifyCertExtensions with partial issuer match", func(t *testing.T) {
		err := VerifyCertExtensions(results, "", "owner", "owner/repo", "https://token.actions.githubusercontent.com")
		require.ErrorContains(t, err, "expected Issuer to be https://token.actions.githubusercontent.com, got https://token.actions.githubusercontent.com/foo-bar -- if you have a custom OIDC issuer")
	})

	t.Run("VerifyCertExtensions with wrong issuer", func(t *testing.T) {
		err := VerifyCertExtensions(results, "", "owner", "", "wrong")
		require.ErrorContains(t, err, "expected Issuer to be wrong, got https://token.actions.githubusercontent.com/foo-bar")
	})
}

func TestVerifyTenancyCertExtensions(t *testing.T) {
	defaultIssuer := GitHubOIDCIssuer

	results := []*AttestationProcessingResult{
		{
			VerificationResult: &verify.VerificationResult{
				Signature: &verify.SignatureVerificationResult{
					Certificate: &certificate.Summary{
						Extensions: certificate.Extensions{
							SourceRepositoryOwnerURI: "https://foo.ghe.com/owner",
							SourceRepositoryURI:      "https://foo.ghe.com/owner/repo",
							Issuer:                   "https://token.actions.foo.ghe.com",
						},
					},
				},
			},
		},
	}

	t.Run("VerifyCertExtensions with owner and repo", func(t *testing.T) {
		err := VerifyCertExtensions(results, "foo", "owner", "owner/repo", defaultIssuer)
		require.NoError(t, err)
	})

	t.Run("VerifyCertExtensions with owner and repo, no tenant", func(t *testing.T) {
		err := VerifyCertExtensions(results, "", "owner", "owner/repo", defaultIssuer)
		require.ErrorContains(t, err, "expected SourceRepositoryOwnerURI to be https://github.com/owner, got https://foo.ghe.com/owner")
	})

	t.Run("VerifyCertExtensions with owner and repo, wrong tenant", func(t *testing.T) {
		err := VerifyCertExtensions(results, "bar", "owner", "owner/repo", defaultIssuer)
		require.ErrorContains(t, err, "expected SourceRepositoryOwnerURI to be https://bar.ghe.com/owner, got https://foo.ghe.com/owner")
	})

	t.Run("VerifyCertExtensions with owner", func(t *testing.T) {
		err := VerifyCertExtensions(results, "foo", "owner", "", defaultIssuer)
		require.NoError(t, err)
	})

	t.Run("VerifyCertExtensions with wrong owner", func(t *testing.T) {
		err := VerifyCertExtensions(results, "foo", "wrong", "", defaultIssuer)
		require.ErrorContains(t, err, "expected SourceRepositoryOwnerURI to be https://foo.ghe.com/wrong, got https://foo.ghe.com/owner")
	})

	t.Run("VerifyCertExtensions with wrong repo", func(t *testing.T) {
		err := VerifyCertExtensions(results, "foo", "owner", "wrong", defaultIssuer)
		require.ErrorContains(t, err, "expected SourceRepositoryURI to be https://foo.ghe.com/wrong, got https://foo.ghe.com/owner/repo")
	})

	t.Run("VerifyCertExtensions with correct, non-default issuer", func(t *testing.T) {
		err := VerifyCertExtensions(results, "foo", "owner", "owner/repo", "https://token.actions.foo.ghe.com")
		require.NoError(t, err)
	})

	t.Run("VerifyCertExtensions with wrong issuer", func(t *testing.T) {
		err := VerifyCertExtensions(results, "foo", "owner", "owner/repo", "wrong")
		require.ErrorContains(t, err, "expected Issuer to be wrong, got https://token.actions.foo.ghe.com")
	})
}
