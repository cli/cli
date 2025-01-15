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
	"github.com/cli/go-gh/v2/pkg/auth"
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

	host, _ := auth.DefaultHost()

	publicGoodOpts := Options{
		APIClient:        api.NewLiveClient(hc, host, logger),
		ArtifactPath:     artifactPath,
		BundlePath:       bundlePath,
		DigestAlgorithm:  "sha512",
		Logger:           logger,
		OCIClient:        oci.NewLiveClient(),
		OIDCIssuer:       verification.GitHubOIDCIssuer,
		Owner:            "sigstore",
		PredicateType:    verification.SLSAPredicateV1,
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
		require.ErrorContains(t, err, "expected SourceRepositoryURI to be https://github.com/sigstore/fakerepo, got https://github.com/sigstore/sigstore-js")
	})

	t.Run("with invalid owner", func(t *testing.T) {
		opts := publicGoodOpts
		opts.Owner = "fakeowner"

		err := runVerify(&opts)
		require.Error(t, err)
		require.ErrorContains(t, err, "expected SourceRepositoryOwnerURI to be https://github.com/fakeowner, got https://github.com/sigstore")
	})

	t.Run("with no matching OIDC issuer", func(t *testing.T) {
		opts := publicGoodOpts
		opts.OIDCIssuer = "some-other-issuer"

		err := runVerify(&opts)
		require.Error(t, err)
		require.ErrorContains(t, err, "expected Issuer to be some-other-issuer, got https://token.actions.githubusercontent.com")
	})

	t.Run("with invalid SAN", func(t *testing.T) {
		opts := publicGoodOpts
		opts.SAN = "fake san"

		err := runVerify(&opts)
		require.Error(t, err)
		require.ErrorContains(t, err, "verifying with issuer \"sigstore.dev\"")
	})

	t.Run("with invalid SAN regex", func(t *testing.T) {
		opts := publicGoodOpts
		opts.SANRegex = "^https://github.com/sigstore/not-real/"

		err := runVerify(&opts)
		require.Error(t, err)
		require.ErrorContains(t, err, "verifying with issuer \"sigstore.dev\"")
	})

	t.Run("with bundle from OCI registry", func(t *testing.T) {
		opts := Options{
			APIClient:             api.NewLiveClient(hc, host, logger),
			ArtifactPath:          "oci://ghcr.io/github/artifact-attestations-helm-charts/policy-controller:v0.10.0-github9",
			UseBundleFromRegistry: true,
			DigestAlgorithm:       "sha256",
			Logger:                logger,
			OCIClient:             oci.NewLiveClient(),
			OIDCIssuer:            verification.GitHubOIDCIssuer,
			Owner:                 "github",
			PredicateType:         verification.SLSAPredicateV1,
			SANRegex:              "^https://github.com/github/",
			SigstoreVerifier:      verification.NewLiveSigstoreVerifier(sigstoreConfig),
		}

		err := runVerify(&opts)
		require.NoError(t, err)
	})
}

func TestVerifyIntegrationCustomIssuer(t *testing.T) {
	artifactPath := test.NormalizeRelativePath("../test/data/custom-issuer-artifact")
	bundlePath := test.NormalizeRelativePath("../test/data/custom-issuer.sigstore.json")

	logger := io.NewTestHandler()

	sigstoreConfig := verification.SigstoreConfig{
		Logger: logger,
	}

	cmdFactory := factory.New("test")

	hc, err := cmdFactory.HttpClient()
	if err != nil {
		t.Fatal(err)
	}

	host, _ := auth.DefaultHost()

	baseOpts := Options{
		APIClient:        api.NewLiveClient(hc, host, logger),
		ArtifactPath:     artifactPath,
		BundlePath:       bundlePath,
		DigestAlgorithm:  "sha256",
		Logger:           logger,
		OCIClient:        oci.NewLiveClient(),
		OIDCIssuer:       "https://token.actions.githubusercontent.com/hammer-time",
		PredicateType:    verification.SLSAPredicateV1,
		SigstoreVerifier: verification.NewLiveSigstoreVerifier(sigstoreConfig),
	}

	t.Run("with owner and valid workflow SAN", func(t *testing.T) {
		opts := baseOpts
		opts.Owner = "too-legit"
		opts.SAN = "https://github.com/too-legit/attest/.github/workflows/integration.yml@refs/heads/main"

		err := runVerify(&opts)
		require.NoError(t, err)
	})

	t.Run("with owner and valid workflow SAN regex", func(t *testing.T) {
		opts := baseOpts
		opts.Owner = "too-legit"
		opts.SANRegex = "^https://github.com/too-legit/attest"

		err := runVerify(&opts)
		require.NoError(t, err)
	})

	t.Run("with repo and valid workflow SAN", func(t *testing.T) {
		opts := baseOpts
		opts.Owner = "too-legit"
		opts.Repo = "too-legit/attest"
		opts.SAN = "https://github.com/too-legit/attest/.github/workflows/integration.yml@refs/heads/main"

		err := runVerify(&opts)
		require.NoError(t, err)
	})

	t.Run("with repo and valid workflow SAN regex", func(t *testing.T) {
		opts := baseOpts
		opts.Owner = "too-legit"
		opts.Repo = "too-legit/attest"
		opts.SANRegex = "^https://github.com/too-legit/attest"

		err := runVerify(&opts)
		require.NoError(t, err)
	})
}

func TestVerifyIntegrationReusableWorkflow(t *testing.T) {
	artifactPath := test.NormalizeRelativePath("../test/data/reusable-workflow-artifact")
	bundlePath := test.NormalizeRelativePath("../test/data/reusable-workflow-attestation.sigstore.json")

	logger := io.NewTestHandler()

	sigstoreConfig := verification.SigstoreConfig{
		Logger: logger,
	}

	cmdFactory := factory.New("test")

	hc, err := cmdFactory.HttpClient()
	if err != nil {
		t.Fatal(err)
	}

	host, _ := auth.DefaultHost()

	baseOpts := Options{
		APIClient:        api.NewLiveClient(hc, host, logger),
		ArtifactPath:     artifactPath,
		BundlePath:       bundlePath,
		DigestAlgorithm:  "sha256",
		Logger:           logger,
		OCIClient:        oci.NewLiveClient(),
		OIDCIssuer:       verification.GitHubOIDCIssuer,
		PredicateType:    verification.SLSAPredicateV1,
		SigstoreVerifier: verification.NewLiveSigstoreVerifier(sigstoreConfig),
	}

	t.Run("with owner and valid reusable workflow SAN", func(t *testing.T) {
		opts := baseOpts
		opts.Owner = "malancas"
		opts.SAN = "https://github.com/github/artifact-attestations-workflows/.github/workflows/attest.yml@09b495c3f12c7881b3cc17209a327792065c1a1d"

		err := runVerify(&opts)
		require.NoError(t, err)
	})

	t.Run("with owner and valid reusable workflow SAN regex", func(t *testing.T) {
		opts := baseOpts
		opts.Owner = "malancas"
		opts.SANRegex = "^https://github.com/github/artifact-attestations-workflows/"

		err := runVerify(&opts)
		require.NoError(t, err)
	})

	t.Run("with owner and valid reusable signer repo", func(t *testing.T) {
		opts := baseOpts
		opts.Owner = "malancas"
		opts.SignerRepo = "github/artifact-attestations-workflows"

		err := runVerify(&opts)
		require.NoError(t, err)
	})

	t.Run("with repo and valid reusable workflow SAN", func(t *testing.T) {
		opts := baseOpts
		opts.Owner = "malancas"
		opts.Repo = "malancas/attest-demo"
		opts.SAN = "https://github.com/github/artifact-attestations-workflows/.github/workflows/attest.yml@09b495c3f12c7881b3cc17209a327792065c1a1d"

		err := runVerify(&opts)
		require.NoError(t, err)
	})

	t.Run("with repo and valid reusable workflow SAN regex", func(t *testing.T) {
		opts := baseOpts
		opts.Owner = "malancas"
		opts.Repo = "malancas/attest-demo"
		opts.SANRegex = "^https://github.com/github/artifact-attestations-workflows/"

		err := runVerify(&opts)
		require.NoError(t, err)
	})

	t.Run("with repo and valid reusable signer repo", func(t *testing.T) {
		opts := baseOpts
		opts.Owner = "malancas"
		opts.Repo = "malancas/attest-demo"
		opts.SignerRepo = "github/artifact-attestations-workflows"

		err := runVerify(&opts)
		require.NoError(t, err)
	})
}

func TestVerifyIntegrationReusableWorkflowSignerWorkflow(t *testing.T) {
	artifactPath := test.NormalizeRelativePath("../test/data/reusable-workflow-artifact")
	bundlePath := test.NormalizeRelativePath("../test/data/reusable-workflow-attestation.sigstore.json")

	logger := io.NewTestHandler()

	sigstoreConfig := verification.SigstoreConfig{
		Logger: logger,
	}

	cmdFactory := factory.New("test")

	hc, err := cmdFactory.HttpClient()
	if err != nil {
		t.Fatal(err)
	}

	host, _ := auth.DefaultHost()

	baseOpts := Options{
		APIClient:        api.NewLiveClient(hc, host, logger),
		ArtifactPath:     artifactPath,
		BundlePath:       bundlePath,
		Config:           cmdFactory.Config,
		DigestAlgorithm:  "sha256",
		Logger:           logger,
		OCIClient:        oci.NewLiveClient(),
		OIDCIssuer:       verification.GitHubOIDCIssuer,
		Owner:            "malancas",
		PredicateType:    verification.SLSAPredicateV1,
		Repo:             "malancas/attest-demo",
		SigstoreVerifier: verification.NewLiveSigstoreVerifier(sigstoreConfig),
	}

	type testcase struct {
		name           string
		signerWorkflow string
		expectErr      bool
		host           string
	}

	testcases := []testcase{
		{
			name:           "with invalid signer workflow",
			signerWorkflow: "foo/bar/.github/workflows/attest.yml",
			expectErr:      true,
		},
		{
			name:           "valid signer workflow with host",
			signerWorkflow: "github.com/github/artifact-attestations-workflows/.github/workflows/attest.yml",
			expectErr:      false,
		},
		{
			name:           "valid signer workflow without host (defaults to github.com)",
			signerWorkflow: "github/artifact-attestations-workflows/.github/workflows/attest.yml",
			expectErr:      false,
			host:           "github.com",
		},
	}

	for _, tc := range testcases {
		opts := baseOpts
		opts.SignerWorkflow = tc.signerWorkflow
		opts.Hostname = tc.host

		err := runVerify(&opts)
		if tc.expectErr {
			require.Error(t, err, "expected error for '%s'", tc.name)
		} else {
			require.NoError(t, err, "unexpected error for '%s'", tc.name)
		}
	}
}
