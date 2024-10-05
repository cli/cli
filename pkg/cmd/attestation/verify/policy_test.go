package verify

import (
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
		Owner:        "sigstore",
		SANRegex:     "^https://github.com/sigstore/",
	}

	_, err = buildVerifyPolicy(opts, *artifact)
	require.NoError(t, err)
}

func TestValidateSignerWorkflow(t *testing.T) {
	type testcase struct {
		name                   string
		providedSignerWorkflow string
		expectedWorkflowRegex  string
		host                   string
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
			host:                   "myhost.github.com",
		},
		{
			name:                   "workflow with authenticated host",
			providedSignerWorkflow: "github/artifact-attestations-workflows/.github/workflows/attest.yml",
			expectedWorkflowRegex:  "^https://authedhost.github.com/github/artifact-attestations-workflows/.github/workflows/attest.yml",
			host:                   "authedhost.github.com",
		},
		{
			name:                   "workflow with authenticated host",
			providedSignerWorkflow: "github/artifact-attestations-workflows/.github/workflows/attest.yml",
			expectedWorkflowRegex:  "^https://authedhost.github.com/github/artifact-attestations-workflows/.github/workflows/attest.yml",
			host:                   "authedhost.github.com",
		},
	}

	for _, tc := range testcases {
		cmdFactory := factory.New("test")

		opts := &Options{
			Config:         cmdFactory.Config,
			SignerWorkflow: tc.providedSignerWorkflow,
		}

		// All host resolution is done verify.go:RunE
		if tc.host == "" {
			// Set to default host
			tc.host = "github.com"
		}
		opts.Hostname = tc.host

		workflowRegex, err := validateSignerWorkflow(opts)
		require.NoError(t, err)
		require.Equal(t, tc.expectedWorkflowRegex, workflowRegex)

	}
}

func TestValidateSignerWorkflowNoHost(t *testing.T) {
	cmdFactory := factory.New("test")
	opts := &Options{
		Config:         cmdFactory.Config,
		SignerWorkflow: "github/artifact-attestations-workflows/.github/workflows/attest.yml",
	}

	workflowRegex, err := validateSignerWorkflow(opts)
	require.Error(t, err)
	require.ErrorContains(t, err, "unknown host")
	require.Equal(t, "", workflowRegex)
}
