//go:build integration

package verification

import (
	"testing"

	"github.com/cli/cli/v2/pkg/cmd/attestation/api"
	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact"
	"github.com/cli/cli/v2/pkg/cmd/attestation/io"
	"github.com/cli/cli/v2/pkg/cmd/attestation/test"

	"github.com/sigstore/sigstore-go/pkg/verify"
	"github.com/stretchr/testify/require"
)

func TestLiveSigstoreVerifier(t *testing.T) {
	t.Run("with invalid signature", func(t *testing.T) {
		attestations := getAttestationsFor(t, "../test/data/sigstoreBundle-invalid-signature.json")
		require.NotNil(t, attestations)

		verifier := NewLiveSigstoreVerifier(SigstoreConfig{
			Logger: io.NewTestHandler(),
		})

		res := verifier.Verify(attestations, publicGoodPolicy(t))
		require.Error(t, res.Error)
		require.ErrorContains(t, res.Error, "verifying with issuer \"sigstore.dev\"")
		require.Nil(t, res.VerifyResults)
	})

	t.Run("with valid artifact and JSON lines file containing multiple Sigstore bundles", func(t *testing.T) {
		attestations := getAttestationsFor(t, "../test/data/sigstore-js-2.1.0_with_2_bundles.jsonl")
		require.Len(t, attestations, 2)

		verifier := NewLiveSigstoreVerifier(SigstoreConfig{
			Logger: io.NewTestHandler(),
		})

		res := verifier.Verify(attestations, publicGoodPolicy(t))
		require.Len(t, res.VerifyResults, 2)
		require.NoError(t, res.Error)
	})

	t.Run("with missing verification material", func(t *testing.T) {
		attestations := getAttestationsFor(t, "../test/data/github_provenance_demo-0.0.12-py3-none-any-bundle-missing-verification-material.jsonl")
		require.NotNil(t, attestations)

		verifier := NewLiveSigstoreVerifier(SigstoreConfig{
			Logger: io.NewTestHandler(),
		})

		res := verifier.Verify(attestations, publicGoodPolicy(t))
		require.Error(t, res.Error)
		require.ErrorContains(t, res.Error, "failed to get bundle verification content")
		require.Nil(t, res.VerifyResults)
	})

	t.Run("with missing verification certificate", func(t *testing.T) {
		attestations := getAttestationsFor(t, "../test/data/github_provenance_demo-0.0.12-py3-none-any-bundle-missing-cert.jsonl")
		require.NotNil(t, attestations)

		verifier := NewLiveSigstoreVerifier(SigstoreConfig{
			Logger: io.NewTestHandler(),
		})

		res := verifier.Verify(attestations, publicGoodPolicy(t))
		require.Error(t, res.Error)
		require.ErrorContains(t, res.Error, "leaf cert not found")
		require.Nil(t, res.VerifyResults)
	})

	t.Run("with GitHub Sigstore artifact", func(t *testing.T) {
		githubArtifactPath := test.NormalizeRelativePath("../test/data/github_provenance_demo-0.0.12-py3-none-any.whl")
		githubArtifact, err := artifact.NewDigestedArtifact(nil, githubArtifactPath, "sha256")
		require.NoError(t, err)

		githubPolicy := buildPolicy(t, *githubArtifact)

		attestations := getAttestationsFor(t, "../test/data/github_provenance_demo-0.0.12-py3-none-any-bundle.jsonl")

		verifier := NewLiveSigstoreVerifier(SigstoreConfig{
			Logger: io.NewTestHandler(),
		})

		res := verifier.Verify(attestations, githubPolicy)
		require.Len(t, res.VerifyResults, 1)
		require.NoError(t, res.Error)
	})

	t.Run("with custom trusted root", func(t *testing.T) {
		attestations := getAttestationsFor(t, "../test/data/sigstore-js-2.1.0_with_2_bundles.jsonl")

		verifier := NewLiveSigstoreVerifier(SigstoreConfig{
			Logger:      io.NewTestHandler(),
			TrustedRoot: test.NormalizeRelativePath("../test/data/trusted_root.json"),
		})

		res := verifier.Verify(attestations, publicGoodPolicy(t))
		require.Len(t, res.VerifyResults, 2)
		require.NoError(t, res.Error)
	})

	t.Run("with invalid bundle version", func(t *testing.T) {
		attestations := getAttestationsFor(t, "../test/data/sigstore-js-2.1.0-bundle-v0.1.json")
		require.Len(t, attestations, 1)

		verifier := NewLiveSigstoreVerifier(SigstoreConfig{
			Logger: io.NewTestHandler(),
		})

		res := verifier.Verify(attestations, publicGoodPolicy(t))
		require.Len(t, res.VerifyResults, 0)
		require.ErrorContains(t, res.Error, "unsupported bundle version")
	})
}

func publicGoodPolicy(t *testing.T) verify.PolicyBuilder {
	t.Helper()

	artifactPath := test.NormalizeRelativePath("../test/data/sigstore-js-2.1.0.tgz")
	publicGoodArtifact, err := artifact.NewDigestedArtifact(nil, artifactPath, "sha512")
	require.NoError(t, err)

	return buildPolicy(t, *publicGoodArtifact)
}

func buildPolicy(t *testing.T, artifact artifact.DigestedArtifact) verify.PolicyBuilder {
	t.Helper()

	artifactDigestPolicyOption, err := BuildDigestPolicyOption(artifact)
	require.NoError(t, err)

	return verify.NewPolicy(artifactDigestPolicyOption, verify.WithoutIdentitiesUnsafe())
}

func getAttestationsFor(t *testing.T, bundlePath string) []*api.Attestation {
	t.Helper()

	attestations, err := GetLocalAttestations(bundlePath)
	require.NoError(t, err)

	return attestations
}
