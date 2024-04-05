package verification

import (
	"testing"

	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact"
	"github.com/cli/cli/v2/pkg/cmd/attestation/io"
	"github.com/cli/cli/v2/pkg/cmd/attestation/test"

	"github.com/sigstore/sigstore-go/pkg/verify"
	"github.com/stretchr/testify/require"
)

func buildPolicy(a artifact.DigestedArtifact) (verify.PolicyBuilder, error) {
	artifactDigestPolicyOption, err := BuildDigestPolicyOption(a)
	if err != nil {
		return verify.PolicyBuilder{}, err
	}

	policy := verify.NewPolicy(artifactDigestPolicyOption, verify.WithoutIdentitiesUnsafe())
	return policy, nil
}

func TestNewLiveSigstoreVerifier(t *testing.T) {
	artifactPath := test.NormalizeRelativePath("../test/data/sigstore-js-2.1.0.tgz")
	artifact, err := artifact.NewDigestedArtifact(nil, artifactPath, "sha512")
	require.NoError(t, err)

	policy, err := buildPolicy(*artifact)
	require.NoError(t, err)

	c := SigstoreConfig{
		Logger: io.NewTestHandler(),
	}
	verifier, err := NewLiveSigstoreVerifier(c)
	require.NoError(t, err)

	t.Run("with invalid signature", func(t *testing.T) {
		bundlePath := test.NormalizeRelativePath("../test/data/sigstoreBundle-invalid-signature.json")
		attestations, err := GetLocalAttestations(bundlePath)
		require.NotNil(t, attestations)
		require.NoError(t, err)

		res := verifier.Verify(attestations, policy)
		require.Error(t, res.Error)
		require.ErrorContains(t, res.Error, "verifying with issuer \"sigstore.dev\"")
		require.Nil(t, res.VerifyResults)
	})

	t.Run("with valid artifact and JSON lines file containing multiple Sigstore bundles", func(t *testing.T) {
		bundlePath := test.NormalizeRelativePath("../test/data/sigstore-js-2.1.0_with_2_bundles.jsonl")
		attestations, err := GetLocalAttestations(bundlePath)
		require.Len(t, attestations, 2)
		require.NoError(t, err)

		res := verifier.Verify(attestations, policy)
		require.Len(t, res.VerifyResults, 2)
		require.NoError(t, res.Error)
	})
}
