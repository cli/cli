package verification

import (
	"testing"

	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact"
	"github.com/cli/cli/v2/pkg/cmd/attestation/logging"
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

func TestNewSigstoreVerifier(t *testing.T) {
	artifactPath := test.NormalizeRelativePath("../test/data/sigstore-js-2.1.0.tgz")

	t.Run("with invalid signature", func(t *testing.T) {
		artifact, err := artifact.NewDigestedArtifact(nil, artifactPath, "sha512")
		require.NoError(t, err)

		bundlePath := test.NormalizeRelativePath("../test/data/sigstoreBundle-invalid-signature.json")
		attestations, err := GetLocalAttestations(bundlePath)
		require.NotNil(t, attestations)
		require.NoError(t, err)

		policy, err := buildPolicy(*artifact)
		require.NoError(t, err)

		c := SigstoreConfig{
			Logger: logging.NewTestLogger(),
		}
		verifier, err := NewSigstoreVerifier(c, policy)
		require.NoError(t, err)

		res := verifier.Verify(attestations)
		require.Error(t, res.Error)
		require.ErrorContains(t, res.Error, "verifying with issuer \"sigstore.dev\"")
		require.Nil(t, res.VerifyResults)
	})
}
