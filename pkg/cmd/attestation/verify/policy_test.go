package verify

import (
	"testing"

	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact"
	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact/oci"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildPolicy(t *testing.T) {
	ociClient := oci.NewMockClient()
	artifactPath := "../test/data/sigstore-js-2.1.0.tgz"
	digestAlg := "sha256"

	artifact, err := artifact.NewDigestedArtifact(ociClient, artifactPath, digestAlg)
	require.NoError(t, err)

	opts := &Options{
		ArtifactPath:    artifactPath,
		Owner:           "sigstore",
		SANRegex:        "^https://github.com/sigstore/",
	}

	_, err = buildVerifyPolicy(opts, *artifact)
	assert.NoError(t, err)
}
