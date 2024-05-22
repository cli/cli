//go:build integration

package verify

import (
	"testing"

	"github.com/cli/cli/v2/pkg/cmd/attestation/api"
	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact/oci"
	"github.com/cli/cli/v2/pkg/cmd/attestation/io"
	"github.com/cli/cli/v2/pkg/cmd/attestation/test"
	"github.com/cli/cli/v2/pkg/cmd/attestation/verification"
	"github.com/cli/cli/v2/pkg/cmd/factory"
	"github.com/stretchr/testify/require"
)

func TestVerifyIntegration(t *testing.T) {
	logger := io.NewTestHandler()

	sigstoreConfig := verification.SigstoreConfig{
		Logger: logger,
	}

	cmdFactory := factory.New("test")

	hc, err := cmdFactory.HttpClient()
	if err != nil {
		t.Fatal(err)
	}

	publicGoodOpts := Options{
		APIClient:        api.NewLiveClient(hc, logger),
		ArtifactPath:     artifactPath,
		BundlePath:       bundlePath,
		DigestAlgorithm:  "sha512",
		Logger:           logger,
		OCIClient:        oci.NewLiveClient(),
		OIDCIssuer:       GitHubOIDCIssuer,
		Owner:            "sigstore",
		SANRegex:         "^https://github.com/sigstore/",
		SigstoreVerifier: verification.NewLiveSigstoreVerifier(sigstoreConfig),
	}

	t.Run("with valid owner", func(t *testing.T) {
		err := runVerify(&publicGoodOpts)
		require.NoError(t, err)
	})

	t.Run("with valid repo", func(t *testing.T) {
		opts := publicGoodOpts
		opts.Repo = "sigstore/sigstore-js"

		err := runVerify(&opts)
		require.NoError(t, err)
	})

	t.Run("with valid owner and invalid repo", func(t *testing.T) {
		opts := publicGoodOpts
		opts.Repo = "sigstore/fakerepo"

		err := runVerify(&opts)
		require.Error(t, err)
		require.ErrorContains(t, err, "verifying with issuer \"sigstore.dev\": failed to verify certificate identity: no matching certificate identity found")
	})

	t.Run("with invalid owner", func(t *testing.T) {
		opts := publicGoodOpts
		opts.Owner = "fakeowner"

		err := runVerify(&opts)
		require.Error(t, err)
		require.ErrorContains(t, err, "verifying with issuer \"sigstore.dev\": failed to verify certificate identity: no matching certificate identity found")
	})

	t.Run("with invalid owner and invalid repo", func(t *testing.T) {
		opts := publicGoodOpts
		opts.Repo = "fakeowner/fakerepo"

		err := runVerify(&opts)
		require.Error(t, err)
		require.ErrorContains(t, err, "verifying with issuer \"sigstore.dev\": failed to verify certificate identity: no matching certificate identity found")
	})

	t.Run("with owner and valid cert-identity-regex", func(t *testing.T) {
		artifactPath := test.NormalizeRelativePath("../test/data/shared-workflow-artifact")
		bundlePath := test.NormalizeRelativePath("../test/data/shared-workflow-attestation.sigstore.json")

		opts := Options{
			APIClient:        api.NewLiveClient(hc, logger),
			ArtifactPath:     artifactPath,
			BundlePath:       bundlePath,
			DigestAlgorithm:  "sha256",
			Logger:           logger,
			OCIClient:        oci.NewLiveClient(),
			OIDCIssuer:       GitHubOIDCIssuer,
			Owner:            "malancas",
			SANRegex:         "^https://github.com/bdehamer/workflows/",
			SigstoreVerifier: verification.NewLiveSigstoreVerifier(sigstoreConfig),
		}

		err := runVerify(&opts)
		require.NoError(t, err)
	})

	t.Run("with repo and valid cert-identity-regex", func(t *testing.T) {
		artifactPath := test.NormalizeRelativePath("../test/data/shared-workflow-artifact")
		bundlePath := test.NormalizeRelativePath("../test/data/shared-workflow-attestation.sigstore.json")

		opts := Options{
			APIClient:        api.NewLiveClient(hc, logger),
			ArtifactPath:     artifactPath,
			BundlePath:       bundlePath,
			DigestAlgorithm:  "sha256",
			Logger:           logger,
			OCIClient:        oci.NewLiveClient(),
			OIDCIssuer:       GitHubOIDCIssuer,
			Owner:            "malancas",
			Repo:             "malancas/attest-demo",
			SANRegex:         "^https://github.com/bdehamer/workflows/",
			SigstoreVerifier: verification.NewLiveSigstoreVerifier(sigstoreConfig),
		}

		err := runVerify(&opts)
		require.NoError(t, err)
	})

	t.Run("with repo and valid cert-identity", func(t *testing.T) {
		artifactPath := test.NormalizeRelativePath("../test/data/shared-workflow-artifact")
		bundlePath := test.NormalizeRelativePath("../test/data/shared-workflow-attestation.sigstore.json")

		opts := Options{
			APIClient:        api.NewLiveClient(hc, logger),
			ArtifactPath:     artifactPath,
			BundlePath:       bundlePath,
			DigestAlgorithm:  "sha256",
			Logger:           logger,
			OCIClient:        oci.NewLiveClient(),
			OIDCIssuer:       GitHubOIDCIssuer,
			Owner:            "malancas",
			Repo:             "malancas/attest-demo",
			SAN:              "https://github.com/bdehamer/workflows/.github/workflows/attest.yml@refs/heads/main",
			SigstoreVerifier: verification.NewLiveSigstoreVerifier(sigstoreConfig),
		}

		err := runVerify(&opts)
		require.NoError(t, err)
	})

	t.Run("with owner and valid cert-identity", func(t *testing.T) {
		artifactPath := test.NormalizeRelativePath("../test/data/shared-workflow-artifact")
		bundlePath := test.NormalizeRelativePath("../test/data/shared-workflow-attestation.sigstore.json")

		opts := Options{
			APIClient:        api.NewLiveClient(hc, logger),
			ArtifactPath:     artifactPath,
			BundlePath:       bundlePath,
			DigestAlgorithm:  "sha256",
			Logger:           logger,
			OCIClient:        oci.NewLiveClient(),
			OIDCIssuer:       GitHubOIDCIssuer,
			Owner:            "malancas",
			SAN:              "https://github.com/bdehamer/workflows/.github/workflows/attest.yml@refs/heads/main",
			SigstoreVerifier: verification.NewLiveSigstoreVerifier(sigstoreConfig),
		}

		err := runVerify(&opts)
		require.NoError(t, err)
	})

	t.Run("with owner and valid cert-identity-regex", func(t *testing.T) {
		artifactPath := test.NormalizeRelativePath("../test/data/shared-workflow-artifact")
		bundlePath := test.NormalizeRelativePath("../test/data/shared-workflow-attestation.sigstore.json")

		opts := Options{
			APIClient:        api.NewLiveClient(hc, logger),
			ArtifactPath:     artifactPath,
			BundlePath:       bundlePath,
			DigestAlgorithm:  "sha256",
			Logger:           logger,
			OCIClient:        oci.NewLiveClient(),
			OIDCIssuer:       GitHubOIDCIssuer,
			Owner:            "malancas",
			SANRegex:         "^https://github.com/bdehamer/workflows/",
			SigstoreVerifier: verification.NewLiveSigstoreVerifier(sigstoreConfig),
		}

		err := runVerify(&opts)
		require.NoError(t, err)
	})
}
