package verify

import (
	"testing"

	"github.com/github/gh-attestation/pkg/artifact"
	"github.com/github/gh-attestation/pkg/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildPolicy(t *testing.T) {
	opts := &VerifyOpts{
		ArtifactPath: "../../test/data/public-good/sigstore-js-2.1.0.tgz",
		BundlePath:   "../../test/data/public-good/sigstore-js-2.1.0-bundle.json",
		GitHubClient: github.NewTestClient(),
		OIDCIssuer:   GitHubOIDCIssuer,
		SAN:          "fake san",
		Owner:        "github",
	}

	artifact, err := artifact.NewDigestedArtifact(opts.OCIClient, opts.ArtifactPath, opts.DigestAlgorithm)
	require.NoError(t, err)

	_, err = buildVerifyPolicy(opts, artifact.Digest())
	assert.NoError(t, err)
}
