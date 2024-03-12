package inspect

import (
	"testing"

	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact/oci"
	"github.com/cli/cli/v2/pkg/cmd/attestation/logging"
	"github.com/cli/cli/v2/pkg/cmd/attestation/test"

	"github.com/stretchr/testify/require"
)

const (
	SigstoreSanValue = "https://github.com/sigstore/sigstore-js/.github/workflows/release.yml@refs/heads/main"
	SigstoreSanRegex = "^https://github.com/sigstore/sigstore-js/"
)

var (
	artifactPath = test.NormalizeRelativePath("../test/data/sigstore-js-2.1.0.tgz")
	bundlePath   = test.NormalizeRelativePath("../test/data/sigstore-js-2.1.0-bundle.json")
)

func TestRunInspect(t *testing.T) {
	opts := Options{
		ArtifactPath:    artifactPath,
		BundlePath:      bundlePath,
		DigestAlgorithm: "sha512",
		Logger:          logging.NewTestLogger(),
		OCIClient:       oci.MockClient{},
	}

	t.Run("with valid artifact and bundle", func(t *testing.T) {
		require.Nil(t, RunInspect(&opts))
	})

	t.Run("with missing artifact path", func(t *testing.T) {
		customOpts := opts
		customOpts.ArtifactPath = "../test/data/non-existent-artifact.zip"
		require.Error(t, RunInspect(&customOpts))
	})

	t.Run("with missing bundle path", func(t *testing.T) {
		customOpts := opts
		customOpts.BundlePath = "../test/data/non-existent-sigstoreBundle.json"
		require.Error(t, RunInspect(&customOpts))
	})

	t.Run("with invalid signature", func(t *testing.T) {
		customOpts := opts
		customOpts.BundlePath = "../test/data/sigstoreBundle-invalid-signature.json"

		err := RunInspect(&customOpts)
		require.Error(t, err)
		require.ErrorContains(t, err, "at least one attestation failed to verify")
		require.ErrorContains(t, err, "verifying with issuer \"sigstore.dev\"")
	})

	t.Run("with valid artifact and JSON lines file containing multiple bundles", func(t *testing.T) {
		customOpts := opts
		customOpts.BundlePath = "../test/data/sigstore-js-2.1.0_with_2_bundles.jsonl"
		require.Nil(t, RunInspect(&customOpts))
	})

	t.Run("with missing OCI client", func(t *testing.T) {
		customOpts := opts
		customOpts.ArtifactPath = "oci://ghcr.io/github/test"
		customOpts.OCIClient = nil
		require.Error(t, RunInspect(&customOpts))
	})
}
