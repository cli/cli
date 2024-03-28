package verify

import (
	"testing"

	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact"
	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact/oci"

	"github.com/stretchr/testify/require"
)

// This tests that a policy can be built from a valid artifact
// Note that policy use is tested in verify_test.go in this package
func TestBuildPolicy(t *testing.T) {
	ociClient := oci.MockClient{}
	artifactPath := "../test/data/sigstore-js-2.1.0.tgz"
	digestAlg := "sha256"

	artifact, err := artifact.NewDigestedArtifact(ociClient, artifactPath, digestAlg)
	require.NoError(t, err)

	opts := &Options{
		ArtifactPath: artifactPath,
		OIDCIssuer:   GitHubOIDCIssuer,
		Owner:        "sigstore",
		SANRegex:     "^https://github.com/sigstore/",
	}

	_, err = buildVerifyPolicy(opts, *artifact)
	require.NoError(t, err)
}
