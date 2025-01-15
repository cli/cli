package inspect

import (
	"testing"

	"github.com/cli/cli/v2/pkg/cmd/attestation/test"
	"github.com/cli/cli/v2/pkg/cmd/attestation/verification"

	"github.com/stretchr/testify/require"
)

func TestGetOrgAndRepo(t *testing.T) {
	t.Run("with valid source URL", func(t *testing.T) {
		sourceURL := "https://github.com/github/gh-attestation"
		org, repo, err := getOrgAndRepo("", sourceURL)
		require.Nil(t, err)
		require.Equal(t, "github", org)
		require.Equal(t, "gh-attestation", repo)
	})

	t.Run("with invalid source URL", func(t *testing.T) {
		sourceURL := "hub.com/github/gh-attestation"
		org, repo, err := getOrgAndRepo("", sourceURL)
		require.Error(t, err)
		require.Zero(t, org)
		require.Zero(t, repo)
	})

	t.Run("with valid source tenant URL", func(t *testing.T) {
		sourceURL := "https://foo.ghe.com/github/gh-attestation"
		org, repo, err := getOrgAndRepo("foo", sourceURL)
		require.Nil(t, err)
		require.Equal(t, "github", org)
		require.Equal(t, "gh-attestation", repo)
	})
}

func TestGetAttestationDetail(t *testing.T) {
	bundlePath := test.NormalizeRelativePath("../test/data/sigstore-js-2.1.0-bundle.json")

	attestations, err := verification.GetLocalAttestations(bundlePath)
	require.Len(t, attestations, 1)
	require.NoError(t, err)

	attestation := attestations[0]
	detail, err := getAttestationDetail("", *attestation)
	require.NoError(t, err)

	require.Equal(t, "sigstore", detail.OrgName)
	require.Equal(t, "71096353", detail.OrgID)
	require.Equal(t, "sigstore-js", detail.RepositoryName)
	require.Equal(t, "495574555", detail.RepositoryID)
	require.Equal(t, "https://github.com/sigstore/sigstore-js/actions/runs/6014488666/attempts/1", detail.WorkflowID)
}
