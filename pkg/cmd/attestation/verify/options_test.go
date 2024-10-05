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

func TestAreFlagsValid(t *testing.T) {
	t.Run("has invalid Repo value", func(t *testing.T) {
		opts := Options{
			ArtifactPath:    publicGoodArtifactPath,
			DigestAlgorithm: "sha512",
			OIDCIssuer:      "some issuer",
			Repo:            "sigstoresigstore-js",
		}

		err := opts.AreFlagsValid()
		require.Error(t, err)
		require.ErrorContains(t, err, "invalid value provided for repo")
	})

	t.Run("invalid limit < 0", func(t *testing.T) {
		opts := Options{
			ArtifactPath:    publicGoodArtifactPath,
			BundlePath:      publicGoodBundlePath,
			DigestAlgorithm: "sha512",
			Owner:           "sigstore",
			OIDCIssuer:      "some issuer",
			Limit:           0,
		}

		err := opts.AreFlagsValid()
		require.Error(t, err)
		require.ErrorContains(t, err, "limit 0 not allowed, must be between 1 and 1000")
	})

	t.Run("invalid limit > 1000", func(t *testing.T) {
		opts := Options{
			ArtifactPath:    publicGoodArtifactPath,
			BundlePath:      publicGoodBundlePath,
			DigestAlgorithm: "sha512",
			Owner:           "sigstore",
			OIDCIssuer:      "some issuer",
			Limit:           1001,
		}

		err := opts.AreFlagsValid()
		require.Error(t, err)
		require.ErrorContains(t, err, "limit 1001 not allowed, must be between 1 and 1000")
	})
}

func TestSetPolicyFlags(t *testing.T) {
	t.Run("sets Owner and SANRegex when Repo is provided", func(t *testing.T) {
		opts := Options{
			ArtifactPath:    publicGoodArtifactPath,
			DigestAlgorithm: "sha512",
			OIDCIssuer:      "some issuer",
			Repo:            "sigstore/sigstore-js",
		}

		opts.SetPolicyFlags()
		require.Equal(t, "sigstore", opts.Owner)
		require.Equal(t, "sigstore/sigstore-js", opts.Repo)
		require.Equal(t, "(?i)^https://github.com/sigstore/sigstore-js/", opts.SANRegex)
	})

	t.Run("does not set SANRegex when SANRegex and Repo are provided", func(t *testing.T) {
		opts := Options{
			ArtifactPath:    publicGoodArtifactPath,
			DigestAlgorithm: "sha512",
			OIDCIssuer:      "some issuer",
			Repo:            "sigstore/sigstore-js",
			SANRegex:        "^https://github/foo",
		}

		opts.SetPolicyFlags()
		require.Equal(t, "sigstore", opts.Owner)
		require.Equal(t, "sigstore/sigstore-js", opts.Repo)
		require.Equal(t, "^https://github/foo", opts.SANRegex)
	})

	t.Run("sets SANRegex when Owner is provided", func(t *testing.T) {
		opts := Options{
			ArtifactPath:    publicGoodArtifactPath,
			BundlePath:      publicGoodBundlePath,
			DigestAlgorithm: "sha512",
			OIDCIssuer:      "some issuer",
			Owner:           "sigstore",
		}

		opts.SetPolicyFlags()
		require.Equal(t, "sigstore", opts.Owner)
		require.Equal(t, "(?i)^https://github.com/sigstore/", opts.SANRegex)
	})

	t.Run("does not set SANRegex when SANRegex and Owner are provided", func(t *testing.T) {
		opts := Options{
			ArtifactPath:    publicGoodArtifactPath,
			BundlePath:      publicGoodBundlePath,
			DigestAlgorithm: "sha512",
			OIDCIssuer:      "some issuer",
			Owner:           "sigstore",
			SANRegex:        "^https://github/foo",
		}

		opts.SetPolicyFlags()
		require.Equal(t, "sigstore", opts.Owner)
		require.Equal(t, "^https://github/foo", opts.SANRegex)
	})

	t.Run("returns error when UseBundleFromRegistry is true and ArtifactPath is not an OCI path", func(t *testing.T) {
		opts := Options{
			ArtifactPath:          publicGoodArtifactPath,
			DigestAlgorithm:       "sha512",
			Owner:                 "sigstore",
			UseBundleFromRegistry: true,
			Limit:                 1,
		}

		err := opts.AreFlagsValid()
		require.Error(t, err)
		require.ErrorContains(t, err, "bundle-from-oci flag can only be used with OCI artifact paths")
	})

	t.Run("does not return error when UseBundleFromRegistry is true and ArtifactPath is an OCI path", func(t *testing.T) {
		opts := Options{
			ArtifactPath:          "oci://sigstore/sigstore-js:2.1.0",
			DigestAlgorithm:       "sha512",
			OIDCIssuer:            "some issuer",
			Owner:                 "sigstore",
			UseBundleFromRegistry: true,
			Limit:                 1,
		}

		err := opts.AreFlagsValid()
		require.NoError(t, err)
	})

	t.Run("returns error when UseBundleFromRegistry is true and BundlePath is provided", func(t *testing.T) {
		opts := Options{
			ArtifactPath:          "oci://sigstore/sigstore-js:2.1.0",
			BundlePath:            publicGoodBundlePath,
			DigestAlgorithm:       "sha512",
			Owner:                 "sigstore",
			UseBundleFromRegistry: true,
			Limit:                 1,
		}

		err := opts.AreFlagsValid()
		require.Error(t, err)
		require.ErrorContains(t, err, "bundle-from-oci flag cannot be used with bundle-path flag")
	})
}
