package verify

import (
	"os"
	"testing"

	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact"
	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact/oci"
	"github.com/cli/cli/v2/pkg/cmd/factory"

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

func ValidateSignerWorkflow(t *testing.T) {
	type testcase struct {
		name                   string
		providedSignerWorkflow string
		expectedWorkflowRegex  string
		ghHost                 string
		authHost               string
	}

	testcases := []testcase{
		{
			name:                   "workflow with no host specified",
			providedSignerWorkflow: "github/artifact-attestations-workflows/.github/workflows/attest.yml",
			expectedWorkflowRegex:  "^https://github.com/github/artifact-attestations-workflows/.github/workflows/attest.yml",
		},
		{
			name:                   "workflow with host specified",
			providedSignerWorkflow: "github.com/github/artifact-attestations-workflows/.github/workflows/attest.yml",
			expectedWorkflowRegex:  "^https://github.com/github/artifact-attestations-workflows/.github/workflows/attest.yml",
		},
		{
			name:                   "workflow with GH_HOST set",
			providedSignerWorkflow: "github/artifact-attestations-workflows/.github/workflows/attest.yml",
			expectedWorkflowRegex:  "^https://myhost.github.com/github/artifact-attestations-workflows/.github/workflows/attest.yml",
			ghHost:                 "myhost.github.com",
		},
		{
			name:                   "workflow with authenticated host",
			providedSignerWorkflow: "github/artifact-attestations-workflows/.github/workflows/attest.yml",
			expectedWorkflowRegex:  "^https://authedhost.github.com/github/artifact-attestations-workflows/.github/workflows/attest.yml",
			authHost:               "authedhost.github.com",
		},
	}

	for _, tc := range testcases {
		cmdFactory := factory.New("test")

		opts := &Options{
			Config:         cmdFactory.Config,
			SignerWorkflow: tc.providedSignerWorkflow,
		}

		if tc.ghHost != "" {
			err := os.Setenv("GH_HOST", tc.ghHost)
			require.NoError(t, err)
		}

		if tc.authHost != "" {
			cfg, err := opts.Config()
			require.NoError(t, err)

			// if authenticated, return the authenticated host
			authCfg := cfg.Authentication()
			authCfg.SetDefaultHost(tc.authHost, "")
		}

		workflowRegex, err := validateSignerWorkflow(opts)
		require.NoError(t, err)
		require.Equal(t, tc.expectedWorkflowRegex, workflowRegex)
	}
}
