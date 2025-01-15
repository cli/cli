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
	type testcase struct {
		name         string
		attestations []*api.Attestation
		expectErr    bool
		errContains  string
	}

	testcases := []testcase{
		{
			name:         "with invalid signature",
			attestations: getAttestationsFor(t, "../test/data/sigstoreBundle-invalid-signature.json"),
			expectErr:    true,
			errContains:  "verifying with issuer \"sigstore.dev\"",
		},
		{
			name:         "with valid artifact and JSON lines file containing multiple Sigstore bundles",
			attestations: getAttestationsFor(t, "../test/data/sigstore-js-2.1.0_with_2_bundles.jsonl"),
		},
		{
			name:         "with invalid bundle version",
			attestations: getAttestationsFor(t, "../test/data/sigstore-js-2.1.0-bundle-v0.1.json"),
			expectErr:    true,
			errContains:  "unsupported bundle version",
		},
		{
			name:         "with no attestations",
			attestations: []*api.Attestation{},
			expectErr:    true,
			errContains:  "no attestations were verified",
		},
	}

	for _, tc := range testcases {
		verifier := NewLiveSigstoreVerifier(SigstoreConfig{
			Logger: io.NewTestHandler(),
		})

		results, err := verifier.Verify(tc.attestations, publicGoodPolicy(t))

		if tc.expectErr {
			require.Error(t, err, "test case: %s", tc.name)
			require.ErrorContains(t, err, tc.errContains, "test case: %s", tc.name)
			require.Nil(t, results, "test case: %s", tc.name)
		} else {
			require.Equal(t, len(tc.attestations), len(results), "test case: %s", tc.name)
			require.NoError(t, err, "test case: %s", tc.name)
		}
	}

	t.Run("with 2/3 verified attestations", func(t *testing.T) {
		verifier := NewLiveSigstoreVerifier(SigstoreConfig{
			Logger: io.NewTestHandler(),
		})

		invalidBundle := getAttestationsFor(t, "../test/data/sigstore-js-2.1.0-bundle-v0.1.json")
		attestations := getAttestationsFor(t, "../test/data/sigstore-js-2.1.0_with_2_bundles.jsonl")
		attestations = append(attestations, invalidBundle[0])
		require.Len(t, attestations, 3)

		results, err := verifier.Verify(attestations, publicGoodPolicy(t))

		require.Len(t, results, 2)
		require.NoError(t, err)
	})

	t.Run("fail with 0/2 verified attestations", func(t *testing.T) {
		verifier := NewLiveSigstoreVerifier(SigstoreConfig{
			Logger: io.NewTestHandler(),
		})

		invalidBundle := getAttestationsFor(t, "../test/data/sigstore-js-2.1.0-bundle-v0.1.json")
		attestations := getAttestationsFor(t, "../test/data/sigstoreBundle-invalid-signature.json")
		attestations = append(attestations, invalidBundle[0])
		require.Len(t, attestations, 2)

		results, err := verifier.Verify(attestations, publicGoodPolicy(t))
		require.Nil(t, results)
		require.Error(t, err)
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

		results, err := verifier.Verify(attestations, githubPolicy)
		require.Len(t, results, 1)
		require.NoError(t, err)
	})

	t.Run("with custom trusted root", func(t *testing.T) {
		attestations := getAttestationsFor(t, "../test/data/sigstore-js-2.1.0_with_2_bundles.jsonl")

		verifier := NewLiveSigstoreVerifier(SigstoreConfig{
			Logger:      io.NewTestHandler(),
			TrustedRoot: test.NormalizeRelativePath("../test/data/trusted_root.json"),
		})

		results, err := verifier.Verify(attestations, publicGoodPolicy(t))
		require.Len(t, results, 2)
		require.NoError(t, err)
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
