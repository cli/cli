package verify

import (
	"testing"

	"github.com/cli/cli/v2/pkg/cmd/attestation/test"

	"github.com/stretchr/testify/require"
)

var (
	publicGoodArtifactPath = test.NormalizeRelativePath("../test/data/sigstore-js-2.1.0.tgz")
	publicGoodBundlePath   = test.NormalizeRelativePath("../test/data/psigstore-js-2.1.0-bundle.json")
)

var baseOptions = Options{
	ArtifactPath:    publicGoodArtifactPath,
	BundlePath:      publicGoodBundlePath,
	DigestAlgorithm: "sha512",
	Limit:           1,
	Owner:           "sigstore",
	OIDCIssuer:      "some issuer",
}

func TestAreFlagsValid(t *testing.T) {
	t.Run("has invalid Repo value", func(t *testing.T) {
		opts := baseOptions
		opts.Repo = "sigstoresigstore-js"

		err := opts.AreFlagsValid()
		require.Error(t, err)
		require.ErrorContains(t, err, "invalid value provided for repo")
	})

	t.Run("invalid limit == 0", func(t *testing.T) {
		opts := baseOptions
		opts.Limit = 0

		err := opts.AreFlagsValid()
		require.Error(t, err)
		require.ErrorContains(t, err, "limit 0 not allowed, must be between 1 and 1000")
	})

	t.Run("invalid limit > 1000", func(t *testing.T) {
		opts := baseOptions
		opts.Limit = 1001

		err := opts.AreFlagsValid()
		require.Error(t, err)
		require.ErrorContains(t, err, "limit 1001 not allowed, must be between 1 and 1000")
	})

	t.Run("returns error when UseBundleFromRegistry is true and ArtifactPath is not an OCI path", func(t *testing.T) {
		opts := baseOptions
		opts.BundlePath = ""
		opts.UseBundleFromRegistry = true

		err := opts.AreFlagsValid()
		require.Error(t, err)
		require.ErrorContains(t, err, "bundle-from-oci flag can only be used with OCI artifact paths")
	})

	t.Run("does not return error when UseBundleFromRegistry is true and ArtifactPath is an OCI path", func(t *testing.T) {
		opts := baseOptions
		opts.ArtifactPath = "oci://sigstore/sigstore-js:2.1.0"
		opts.BundlePath = ""
		opts.UseBundleFromRegistry = true

		err := opts.AreFlagsValid()
		require.NoError(t, err)
	})

	t.Run("returns error when UseBundleFromRegistry is true and BundlePath is provided", func(t *testing.T) {
		opts := baseOptions
		opts.ArtifactPath = "oci://sigstore/sigstore-js:2.1.0"
		opts.UseBundleFromRegistry = true

		err := opts.AreFlagsValid()
		require.Error(t, err)
		require.ErrorContains(t, err, "bundle-from-oci flag cannot be used with bundle-path flag")
	})
}
