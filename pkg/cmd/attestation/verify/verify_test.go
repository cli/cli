package verify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/cli/cli/v2/pkg/cmd/attestation/api"
	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact/oci"
	"github.com/cli/cli/v2/pkg/cmd/attestation/io"
	"github.com/cli/cli/v2/pkg/cmd/attestation/test"
	"github.com/cli/cli/v2/pkg/cmd/attestation/verification"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"

	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"

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

func TestNewVerifyCmd(t *testing.T) {
	testIO, _, _, _ := iostreams.Test()
	var testReg httpmock.Registry
	var metaResp = api.MetaResponse{
		Domains: api.Domain{
			ArtifactAttestations: api.ArtifactAttestations{
				TrustDomain: "foo",
			},
		},
	}
	testReg.Register(httpmock.REST(http.MethodGet, "meta"),
		httpmock.StatusJSONResponse(200, &metaResp))

	f := &cmdutil.Factory{
		IOStreams: testIO,
		HttpClient: func() (*http.Client, error) {
			reg := &testReg
			client := &http.Client{}
			httpmock.ReplaceTripper(client, reg)
			return client, nil
		},
	}

	testcases := []struct {
		name          string
		cli           string
		wants         Options
		wantsErr      bool
		wantsExporter bool
	}{
		{
			name: "Invalid digest-alg flag",
			cli:  fmt.Sprintf("%s --bundle %s --digest-alg sha384 --owner sigstore", artifactPath, bundlePath),
			wants: Options{
				ArtifactPath:     test.NormalizeRelativePath("../test/data/sigstore-js-2.1.0.tgz"),
				BundlePath:       test.NormalizeRelativePath("../test/data/sigstore-js-2.1.0-bundle.json"),
				DigestAlgorithm:  "sha384",
				Hostname:         "github.com",
				Limit:            30,
				OIDCIssuer:       verification.GitHubOIDCIssuer,
				Owner:            "sigstore",
				PredicateType:    verification.SLSAPredicateV1,
				SigstoreVerifier: verification.NewMockSigstoreVerifier(t),
			},
			wantsErr: true,
		},
		{
			name: "Use default digest-alg value",
			cli:  fmt.Sprintf("%s --bundle %s --owner sigstore", artifactPath, bundlePath),
			wants: Options{
				ArtifactPath:     test.NormalizeRelativePath("../test/data/sigstore-js-2.1.0.tgz"),
				BundlePath:       test.NormalizeRelativePath("../test/data/sigstore-js-2.1.0-bundle.json"),
				DigestAlgorithm:  "sha256",
				Hostname:         "github.com",
				Limit:            30,
				OIDCIssuer:       verification.GitHubOIDCIssuer,
				Owner:            "sigstore",
				PredicateType:    verification.SLSAPredicateV1,
				SigstoreVerifier: verification.NewMockSigstoreVerifier(t),
			},
			wantsErr: false,
		},
		{
			name: "Custom host",
			cli:  fmt.Sprintf("%s --bundle %s --owner sigstore --hostname foo.ghe.com", artifactPath, bundlePath),
			wants: Options{
				ArtifactPath:     test.NormalizeRelativePath("../test/data/sigstore-js-2.1.0.tgz"),
				BundlePath:       test.NormalizeRelativePath("../test/data/sigstore-js-2.1.0-bundle.json"),
				DigestAlgorithm:  "sha256",
				Hostname:         "foo.ghe.com",
				Limit:            30,
				OIDCIssuer:       verification.GitHubOIDCIssuer,
				Owner:            "sigstore",
				PredicateType:    verification.SLSAPredicateV1,
				SigstoreVerifier: verification.NewMockSigstoreVerifier(t),
			},
			wantsErr: false,
		},
		{
			name: "Invalid custom host",
			cli:  fmt.Sprintf("%s --bundle %s --owner sigstore --hostname foo.bar.com", artifactPath, bundlePath),
			wants: Options{
				ArtifactPath:     test.NormalizeRelativePath("../test/data/sigstore-js-2.1.0.tgz"),
				BundlePath:       test.NormalizeRelativePath("../test/data/sigstore-js-2.1.0-bundle.json"),
				DigestAlgorithm:  "sha256",
				Hostname:         "foo.ghe.com",
				Limit:            30,
				OIDCIssuer:       verification.GitHubOIDCIssuer,
				Owner:            "sigstore",
				PredicateType:    verification.SLSAPredicateV1,
				SigstoreVerifier: verification.NewMockSigstoreVerifier(t),
			},
			wantsErr: true,
		},
		{
			name: "Use custom digest-alg value",
			cli:  fmt.Sprintf("%s --bundle %s --owner sigstore --digest-alg sha512", artifactPath, bundlePath),
			wants: Options{
				ArtifactPath:     test.NormalizeRelativePath("../test/data/sigstore-js-2.1.0.tgz"),
				BundlePath:       test.NormalizeRelativePath("../test/data/sigstore-js-2.1.0-bundle.json"),
				DigestAlgorithm:  "sha512",
				Hostname:         "github.com",
				Limit:            30,
				OIDCIssuer:       verification.GitHubOIDCIssuer,
				Owner:            "sigstore",
				PredicateType:    verification.SLSAPredicateV1,
				SigstoreVerifier: verification.NewMockSigstoreVerifier(t),
			},
			wantsErr: false,
		},
		{
			name: "Missing owner and repo flags",
			cli:  artifactPath,
			wants: Options{
				ArtifactPath:     test.NormalizeRelativePath("../test/data/sigstore-js-2.1.0.tgz"),
				DigestAlgorithm:  "sha256",
				Hostname:         "github.com",
				Limit:            30,
				OIDCIssuer:       verification.GitHubOIDCIssuer,
				Owner:            "sigstore",
				PredicateType:    verification.SLSAPredicateV1,
				SANRegex:         "(?i)^https://github.com/sigstore/",
				SigstoreVerifier: verification.NewMockSigstoreVerifier(t),
			},
			wantsErr: true,
		},
		{
			name: "Has both owner and repo flags",
			cli:  fmt.Sprintf("%s --owner sigstore --repo sigstore/sigstore-js", artifactPath),
			wants: Options{
				ArtifactPath:     artifactPath,
				DigestAlgorithm:  "sha256",
				Hostname:         "github.com",
				Limit:            30,
				OIDCIssuer:       verification.GitHubOIDCIssuer,
				Owner:            "sigstore",
				PredicateType:    verification.SLSAPredicateV1,
				Repo:             "sigstore/sigstore-js",
				SigstoreVerifier: verification.NewMockSigstoreVerifier(t),
			},
			wantsErr: true,
		},
		{
			name: "Uses default limit flag",
			cli:  fmt.Sprintf("%s --owner sigstore", artifactPath),
			wants: Options{
				ArtifactPath:     artifactPath,
				DigestAlgorithm:  "sha256",
				Hostname:         "github.com",
				Limit:            30,
				OIDCIssuer:       verification.GitHubOIDCIssuer,
				Owner:            "sigstore",
				PredicateType:    verification.SLSAPredicateV1,
				SigstoreVerifier: verification.NewMockSigstoreVerifier(t),
			},
			wantsErr: false,
		},
		{
			name: "Uses custom limit flag",
			cli:  fmt.Sprintf("%s --owner sigstore --limit 101", artifactPath),
			wants: Options{
				ArtifactPath:     artifactPath,
				DigestAlgorithm:  "sha256",
				Hostname:         "github.com",
				Limit:            101,
				OIDCIssuer:       verification.GitHubOIDCIssuer,
				Owner:            "sigstore",
				PredicateType:    verification.SLSAPredicateV1,
				SigstoreVerifier: verification.NewMockSigstoreVerifier(t),
			},
			wantsErr: false,
		},
		{
			name: "Uses invalid limit flag",
			cli:  fmt.Sprintf("%s --owner sigstore --limit 0", artifactPath),
			wants: Options{
				ArtifactPath:     artifactPath,
				DigestAlgorithm:  "sha256",
				Hostname:         "github.com",
				Limit:            0,
				OIDCIssuer:       verification.GitHubOIDCIssuer,
				Owner:            "sigstore",
				PredicateType:    verification.SLSAPredicateV1,
				SANRegex:         "(?i)^https://github.com/sigstore/",
				SigstoreVerifier: verification.NewMockSigstoreVerifier(t),
			},
			wantsErr: true,
		},
		{
			name: "Has both cert-identity and cert-identity-regex flags",
			cli:  fmt.Sprintf("%s --owner sigstore --cert-identity https://github.com/sigstore/ --cert-identity-regex ^https://github.com/sigstore/", artifactPath),
			wants: Options{
				ArtifactPath:     artifactPath,
				DigestAlgorithm:  "sha256",
				Hostname:         "github.com",
				Limit:            30,
				OIDCIssuer:       verification.GitHubOIDCIssuer,
				Owner:            "sigstore",
				PredicateType:    verification.SLSAPredicateV1,
				SAN:              "https://github.com/sigstore/",
				SANRegex:         "(?i)^https://github.com/sigstore/",
				SigstoreVerifier: verification.NewMockSigstoreVerifier(t),
			},
			wantsErr: true,
		},
		{
			name: "Prints output in JSON format",
			cli:  fmt.Sprintf("%s --bundle %s --owner sigstore --format json", artifactPath, bundlePath),
			wants: Options{
				ArtifactPath:     artifactPath,
				BundlePath:       bundlePath,
				DigestAlgorithm:  "sha256",
				Hostname:         "github.com",
				Limit:            30,
				OIDCIssuer:       verification.GitHubOIDCIssuer,
				Owner:            "sigstore",
				PredicateType:    verification.SLSAPredicateV1,
				SigstoreVerifier: verification.NewMockSigstoreVerifier(t),
			},
			wantsExporter: true,
		},
		{
			name: "Use specified predicate type",
			cli:  fmt.Sprintf("%s --bundle %s --owner sigstore --predicate-type https://spdx.dev/Document/v2.3 --format json", artifactPath, bundlePath),
			wants: Options{
				ArtifactPath:     artifactPath,
				BundlePath:       bundlePath,
				DigestAlgorithm:  "sha256",
				Hostname:         "github.com",
				Limit:            30,
				OIDCIssuer:       verification.GitHubOIDCIssuer,
				Owner:            "sigstore",
				PredicateType:    "https://spdx.dev/Document/v2.3",
				SigstoreVerifier: verification.NewMockSigstoreVerifier(t),
			},
			wantsExporter: true,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			var opts *Options
			cmd := NewVerifyCmd(f, func(o *Options) error {
				opts = o
				return nil
			})

			argv := strings.Split(tc.cli, " ")
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			_, err := cmd.ExecuteC()
			if tc.wantsErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			assert.Equal(t, tc.wants.ArtifactPath, opts.ArtifactPath)
			assert.Equal(t, tc.wants.BundlePath, opts.BundlePath)
			assert.Equal(t, tc.wants.DenySelfHostedRunner, opts.DenySelfHostedRunner)
			assert.Equal(t, tc.wants.DigestAlgorithm, opts.DigestAlgorithm)
			assert.Equal(t, tc.wants.Hostname, opts.Hostname)
			assert.Equal(t, tc.wants.Limit, opts.Limit)
			assert.Equal(t, tc.wants.NoPublicGood, opts.NoPublicGood)
			assert.Equal(t, tc.wants.OIDCIssuer, opts.OIDCIssuer)
			assert.Equal(t, tc.wants.Owner, opts.Owner)
			assert.Equal(t, tc.wants.PredicateType, opts.PredicateType)
			assert.Equal(t, tc.wants.Repo, opts.Repo)
			assert.Equal(t, tc.wants.SAN, opts.SAN)
			assert.Equal(t, tc.wants.SANRegex, opts.SANRegex)
			assert.Equal(t, tc.wants.TrustedRoot, opts.TrustedRoot)
			assert.NotNil(t, opts.APIClient)
			assert.NotNil(t, opts.Logger)
			assert.NotNil(t, opts.OCIClient)
			assert.Equal(t, tc.wantsExporter, opts.exporter != nil)
		})
	}
}

func TestVerifyCmdAuthChecks(t *testing.T) {
	f := &cmdutil.Factory{}

	t.Run("by default auth check is required", func(t *testing.T) {
		cmd := NewVerifyCmd(f, func(o *Options) error {
			return nil
		})

		// IsAuthCheckEnabled assumes commands under test are subcommands
		parent := &cobra.Command{Use: "root"}
		parent.AddCommand(cmd)

		require.NoError(t, cmd.ParseFlags([]string{}))
		require.True(t, cmdutil.IsAuthCheckEnabled(cmd), "expected auth check to be required")
	})

	t.Run("when --bundle flag is provided, auth check is not required", func(t *testing.T) {
		cmd := NewVerifyCmd(f, func(o *Options) error {
			return nil
		})

		// IsAuthCheckEnabled assumes commands under test are subcommands
		parent := &cobra.Command{Use: "root"}
		parent.AddCommand(cmd)

		require.NoError(t, cmd.ParseFlags([]string{"--bundle", "not-important"}))
		require.False(t, cmdutil.IsAuthCheckEnabled(cmd), "expected auth check not to be required due to --bundle flag")
	})
}

func TestJSONOutput(t *testing.T) {
	testIO, _, out, _ := iostreams.Test()
	opts := Options{
		ArtifactPath:     artifactPath,
		BundlePath:       bundlePath,
		DigestAlgorithm:  "sha512",
		APIClient:        api.NewTestClient(),
		Logger:           io.NewHandler(testIO),
		OCIClient:        oci.MockClient{},
		OIDCIssuer:       verification.GitHubOIDCIssuer,
		Owner:            "sigstore",
		PredicateType:    verification.SLSAPredicateV1,
		SANRegex:         "^https://github.com/sigstore/",
		SigstoreVerifier: verification.NewMockSigstoreVerifier(t),
		exporter:         cmdutil.NewJSONExporter(),
	}
	require.NoError(t, runVerify(&opts))

	var target []*verification.AttestationProcessingResult
	err := json.Unmarshal(out.Bytes(), &target)
	require.NoError(t, err)
}

func TestRunVerify(t *testing.T) {
	logger := io.NewTestHandler()

	publicGoodOpts := Options{
		ArtifactPath:     artifactPath,
		BundlePath:       bundlePath,
		DigestAlgorithm:  "sha512",
		APIClient:        api.NewTestClient(),
		Logger:           logger,
		OCIClient:        oci.MockClient{},
		OIDCIssuer:       verification.GitHubOIDCIssuer,
		Owner:            "sigstore",
		PredicateType:    verification.SLSAPredicateV1,
		SANRegex:         "^https://github.com/sigstore/",
		SigstoreVerifier: verification.NewMockSigstoreVerifier(t),
	}

	t.Run("with valid artifact and bundle", func(t *testing.T) {
		require.NoError(t, runVerify(&publicGoodOpts))
	})

	t.Run("with failing OCI artifact fetch", func(t *testing.T) {
		opts := publicGoodOpts
		opts.ArtifactPath = "oci://ghcr.io/github/test"
		opts.OCIClient = oci.ReferenceFailClient{}

		err := runVerify(&opts)
		require.Error(t, err)
		require.ErrorContains(t, err, "failed to parse reference")
	})

	t.Run("with missing artifact path", func(t *testing.T) {
		opts := publicGoodOpts
		opts.ArtifactPath = "../test/data/non-existent-artifact.zip"
		require.Error(t, runVerify(&opts))
	})

	t.Run("with missing bundle path", func(t *testing.T) {
		opts := publicGoodOpts
		opts.BundlePath = "../test/data/non-existent-sigstoreBundle.json"
		require.Error(t, runVerify(&opts))
	})

	t.Run("with owner", func(t *testing.T) {
		opts := publicGoodOpts
		opts.BundlePath = ""
		opts.Owner = "sigstore"

		require.Nil(t, runVerify(&opts))
	})

	t.Run("with owner which not matches SourceRepositoryOwnerURI", func(t *testing.T) {
		opts := publicGoodOpts
		opts.BundlePath = ""
		opts.Owner = "owner"

		err := runVerify(&opts)
		require.ErrorContains(t, err, "expected SourceRepositoryOwnerURI to be https://github.com/owner, got https://github.com/sigstore")
	})

	t.Run("with repo", func(t *testing.T) {
		opts := publicGoodOpts
		opts.BundlePath = ""
		opts.Repo = "sigstore/sigstore-js"

		require.Nil(t, runVerify(&opts))
	})

	// Test with bad tenancy
	t.Run("with bad tenancy", func(t *testing.T) {
		opts := publicGoodOpts
		opts.BundlePath = ""
		opts.Repo = "sigstore/sigstore-js"
		opts.Tenant = "foo"

		err := runVerify(&opts)
		require.ErrorContains(t, err, "expected SourceRepositoryOwnerURI to be https://foo.ghe.com/sigstore, got https://github.com/sigstore")
	})

	t.Run("with repo which not matches SourceRepositoryURI", func(t *testing.T) {
		opts := publicGoodOpts
		opts.BundlePath = ""
		opts.Repo = "sigstore/wrong"

		err := runVerify(&opts)
		require.ErrorContains(t, err, "expected SourceRepositoryURI to be https://github.com/sigstore/wrong, got https://github.com/sigstore/sigstore-js")
	})

	t.Run("with invalid repo", func(t *testing.T) {
		opts := publicGoodOpts
		opts.BundlePath = ""
		opts.Repo = "wrong/example"
		opts.APIClient = api.NewFailTestClient()

		err := runVerify(&opts)
		require.Error(t, err)
		require.ErrorContains(t, err, "failed to fetch attestations from wrong/example")
	})

	t.Run("with invalid owner", func(t *testing.T) {
		opts := publicGoodOpts
		opts.BundlePath = ""
		opts.APIClient = api.NewFailTestClient()
		opts.Owner = "wrong-owner"

		err := runVerify(&opts)
		require.Error(t, err)
		require.ErrorContains(t, err, "failed to fetch attestations from wrong-owner")
	})

	t.Run("with missing API client", func(t *testing.T) {
		customOpts := publicGoodOpts
		customOpts.APIClient = nil
		customOpts.BundlePath = ""
		require.Error(t, runVerify(&customOpts))
	})

	t.Run("with valid OCI artifact", func(t *testing.T) {
		customOpts := publicGoodOpts
		customOpts.ArtifactPath = "oci://ghcr.io/github/test"
		customOpts.BundlePath = ""

		require.Nil(t, runVerify(&customOpts))
	})

	t.Run("with valid OCI artifact with UseBundleFromRegistry flag", func(t *testing.T) {
		customOpts := publicGoodOpts
		customOpts.ArtifactPath = "oci://ghcr.io/github/test"
		customOpts.BundlePath = ""
		customOpts.UseBundleFromRegistry = true

		require.Nil(t, runVerify(&customOpts))
	})

	t.Run("with valid OCI artifact with UseBundleFromRegistry flag but no bundle return from registry", func(t *testing.T) {
		customOpts := publicGoodOpts
		customOpts.ArtifactPath = "oci://ghcr.io/github/test"
		customOpts.BundlePath = ""
		customOpts.UseBundleFromRegistry = true
		customOpts.OCIClient = oci.NoAttestationsClient{}

		require.ErrorContains(t, runVerify(&customOpts), "no attestations found in the OCI registry. Retry the command without the --bundle-from-oci flag to check GitHub for the attestation")
	})

	t.Run("with valid OCI artifact with UseBundleFromRegistry flag but fail on fetching bundle from registry", func(t *testing.T) {
		customOpts := publicGoodOpts
		customOpts.ArtifactPath = "oci://ghcr.io/github/test"
		customOpts.BundlePath = ""
		customOpts.UseBundleFromRegistry = true
		customOpts.OCIClient = oci.NoAttestationsClient{}

		require.ErrorContains(t, runVerify(&customOpts), "no attestations found in the OCI registry. Retry the command without the --bundle-from-oci flag to check GitHub for the attestation")
	})
}
