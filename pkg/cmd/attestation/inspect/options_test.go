package inspect

import (
	"testing"

	"github.com/cli/cli/v2/pkg/cmd/attestation/test"

	"github.com/stretchr/testify/require"
)

func TestAreFlagsValid(t *testing.T) {
	artifactPath := test.NormalizeRelativePath("../test/data/public-good/sigstore-js-2.1.0.tgz")
	bundlePath := test.NormalizeRelativePath("../test/data/public-good/sigstore-js-2.1.0-bundle.json")

	t.Run("missing BundlePath", func(t *testing.T) {
		opts := Options{
			ArtifactPath:    artifactPath,
			DigestAlgorithm: "sha512",
		}

		err := opts.AreFlagsValid()
		require.Error(t, err)
		require.ErrorContains(t, err, "bundle must be provided")
	})

	t.Run("missing DigestAlgorithm", func(t *testing.T) {
		opts := Options{
			ArtifactPath: artifactPath,
			BundlePath:   bundlePath,
		}

		err := opts.AreFlagsValid()
		require.Error(t, err)
		require.ErrorContains(t, err, "digest-alg cannot be empty")
	})

	t.Run("invalid DigestAlgorithm", func(t *testing.T) {
		opts := Options{
			ArtifactPath:    artifactPath,
			BundlePath:      bundlePath,
			DigestAlgorithm: "sha1",
		}

		err := opts.AreFlagsValid()
		require.Error(t, err)
		require.ErrorContains(t, err, "invalid digest algorithm 'sha1' provided in digest-alg")
	})
}
