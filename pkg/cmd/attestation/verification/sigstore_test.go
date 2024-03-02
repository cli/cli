package verify

import (
	"testing"

	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact"
	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact/oci"
	"github.com/cli/cli/v2/pkg/cmd/attestation/github"
	"github.com/cli/cli/v2/pkg/cmd/attestation/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var logger = output.NewDefaultLogger()

var publicGoodOpts = Options{
	ArtifactPath:    test.NormalizeRelativePath("../../test/data/public-good/sigstore-js-2.1.0.tgz"),
	BundlePath:      test.NormalizeRelativePath("../../test/data/public-good/sigstore-js-2.1.0-bundle.json"),
	DigestAlgorithm: "sha512",
	GitHubClient:    github.NewTestClient(),
	Logger:          logger,
	OIDCIssuer:      GitHubOIDCIssuer,
	Owner:           "sigstore",
	Limit:           30,
	OCIClient:       oci.NewMockClient(),
	SANRegex:        "^https://github.com/sigstore/",
}

func TestVerify_PublicGoodSuccess(t *testing.T) {
	t.Skip()
	res := test.SuppressAndRestoreOutput()
	defer res()

	artifact, err := artifact.NewDigestedArtifact(publicGoodOpts.OCIClient, publicGoodOpts.ArtifactPath, publicGoodOpts.DigestAlgorithm)
	require.NoError(t, err)

	attestations, err := getAttestations(&publicGoodOpts, artifact.Digest())
	require.NoError(t, err)

	policy, err := buildVerifyPolicy(&publicGoodOpts, artifact.Digest())
	require.NoError(t, err)

	config := SigstoreConfig{
		CustomTrustedRoot: publicGoodOpts.CustomTrustedRoot,
		Logger:            publicGoodOpts.Logger,
		NoPublicGood:      publicGoodOpts.NoPublicGood,
	}

	v, err := NewSigstoreVerifier(config, policy)
	require.NoError(t, err)

	resp := v.Verify(attestations)
	assert.Nil(t, resp.Error)

	verifyResults := resp.VerifyResults
	assert.Len(t, verifyResults, 1)

	result := verifyResults[0]
	assert.NotNil(t, result.VerificationResult)
	assert.Equal(t, attestations[0], result.Attestation)
}

func TestVerify_PublicGoodFail_WithNoPublicGoodFlag(t *testing.T) {
	t.Skip()
	res := test.SuppressAndRestoreOutput()
	defer res()

	opts := publicGoodOpts
	opts.NoPublicGood = true

	err := opts.AreFlagsValid()
	require.NoError(t, err)

	artifact, err := artifact.NewDigestedArtifact(opts.OCIClient, opts.ArtifactPath, opts.DigestAlgorithm)
	require.NoError(t, err)

	attestations, err := getAttestations(&opts, artifact.Digest())
	require.NoError(t, err)

	policy, err := buildVerifyPolicy(&publicGoodOpts, artifact.Digest())
	require.NoError(t, err)

	config := SigstoreConfig{
		CustomTrustedRoot: opts.CustomTrustedRoot,
		Logger:            opts.Logger,
		NoPublicGood:      opts.NoPublicGood,
	}

	v, err := NewSigstoreVerifier(config, policy)
	require.NoError(t, err)

	resp := v.Verify(attestations)
	assert.Nil(t, resp.VerifyResults)
	assert.ErrorContains(t, resp.Error, "verifying with issuer \"sigstore.dev\"")
}

func TestVerify_Failure(t *testing.T) {
	res := test.SuppressAndRestoreOutput()
	defer res()

	opts := publicGoodOpts
	opts.DigestAlgorithm = "sha256"

	err := opts.AreFlagsValid()
	require.NoError(t, err)

	artifact, err := artifact.NewDigestedArtifact(opts.OCIClient, opts.ArtifactPath, opts.DigestAlgorithm)
	require.NoError(t, err)

	attestations, err := getAttestations(&opts, artifact.Digest())
	require.NoError(t, err)

	policy, err := buildVerifyPolicy(&opts, artifact.Digest())
	require.NoError(t, err)

	config := SigstoreConfig{
		CustomTrustedRoot: opts.CustomTrustedRoot,
		Logger:            opts.Logger,
		NoPublicGood:      opts.NoPublicGood,
	}

	v, err := NewSigstoreVerifier(config, policy)
	require.NoError(t, err)

	resp := v.Verify(attestations)
	assert.NotNil(t, resp.Error)
	assert.Nil(t, resp.VerifyResults)
}
