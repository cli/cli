package verification

import (
	"testing"

	"github.com/cli/cli/v2/pkg/cmd/attestation/api"
	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact"
	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact/oci"
	"github.com/cli/cli/v2/pkg/cmd/attestation/logger"
	"github.com/cli/cli/v2/pkg/cmd/attestation/test"

	"github.com/sigstore/sigstore-go/pkg/verify"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	artifactPath = test.NormalizeRelativePath("../test/data/sigstore-js-2.1.0.tgz")
	bundlePath = test.NormalizeRelativePath("../test/data/sigstore-js-2.1.0-bundle.json")
	ociClient = oci.NewMockClient()
)

func TestVerify_PublicGoodSuccess(t *testing.T) {
	res := test.SuppressAndRestoreOutput()
	defer res()

	artifact, err := artifact.NewDigestedArtifact(ociClient, artifactPath, "sha512")
	require.NoError(t, err)

	c := FetchAttestationsConfig{
		APIClient: api.NewTestClient(),
		BundlePath: test.NormalizeRelativePath("../test/data/sigstore-js-2.1.0-bundle.json"),
		Digest: artifact.DigestWithAlg(),
		Owner: "sigstore",
	}

	attestations, err := GetAttestations(c)
	require.NoError(t, err)

	config := SigstoreConfig{
		Logger:            logger.NewDefaultLogger(),
		NoPublicGood:      false,
	}

	v, err := NewSigstoreVerifier(config, verify.PolicyBuilder{})
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
	res := test.SuppressAndRestoreOutput()
	defer res()

	artifact, err := artifact.NewDigestedArtifact(ociClient, artifactPath, "sha512")
	require.NoError(t, err)

	c := FetchAttestationsConfig{
		BundlePath: bundlePath,
		Digest: artifact.Digest(),
		Owner: "sigstore",
	}

	attestations, err := GetAttestations(c)
	require.NoError(t, err)

	config := SigstoreConfig{
		Logger:            logger.NewDefaultLogger(),
		NoPublicGood:      true,
	}

	v, err := NewSigstoreVerifier(config, verify.PolicyBuilder{})
	require.NoError(t, err)

	resp := v.Verify(attestations)
	assert.Nil(t, resp.VerifyResults)
	assert.ErrorContains(t, resp.Error, "verifying with issuer \"sigstore.dev\"")
}

func TestVerify_Failure(t *testing.T) {
	res := test.SuppressAndRestoreOutput()
	defer res()

	artifact, err := artifact.NewDigestedArtifact(ociClient, artifactPath, "sha512")
	require.NoError(t, err)

	c := FetchAttestationsConfig{
		BundlePath: bundlePath,
		Digest: artifact.Digest(),
		Owner: "sigstore",
	}

	attestations, err := GetAttestations(c)
	require.NoError(t, err)

	config := SigstoreConfig{
		Logger:            logger.NewDefaultLogger(),
		NoPublicGood:      true,
	}

	v, err := NewSigstoreVerifier(config, verify.PolicyBuilder{})
	require.NoError(t, err)

	resp := v.Verify(attestations)
	assert.NotNil(t, resp.Error)
	assert.Nil(t, resp.VerifyResults)
}
