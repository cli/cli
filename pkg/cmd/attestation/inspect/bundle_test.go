package inspect

import (
	"testing"

	"github.com/cli/cli/v2/pkg/cmd/attestation/verification"
	"github.com/cli/cli/v2/pkg/cmd/attestation/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetOrgAndRepo(t *testing.T) {
	t.Run("with valid source URL", func(t *testing.T) {
		sourceURL := "https://github.com/github/gh-attestation"
		org, repo, err := getOrgAndRepo(sourceURL)
		assert.Nil(t, err)
		assert.Equal(t, "github", org)
		assert.Equal(t, "gh-attestation", repo)
	})

	t.Run("with invalid source URL", func(t *testing.T) {
		sourceURL := "hub.com/github/gh-attestation"
		org, repo, err := getOrgAndRepo(sourceURL)
		assert.Error(t, err)
		assert.Zero(t, org)
		assert.Zero(t, repo)
	})
}

func TestGetAttestationDetail(t *testing.T) {
	bundlePath := test.NormalizeRelativePath("../test/data/sigstore-js-2.1.0-bundle.json")

	attestations, err := verification.GetLocalAttestations(bundlePath)
	require.Len(t, attestations, 1)
	require.NoError(t, err)

	attestation := attestations[0]
	detail, err := getAttestationDetail(*attestation)
	assert.NoError(t, err)

	assert.Equal(t, "sigstore", detail.OrgName)
	assert.Equal(t, "71096353", detail.OrgID)
	assert.Equal(t, "sigstore-js", detail.RepositoryName)
	assert.Equal(t, "495574555", detail.RepositoryID)
	assert.Equal(t, "https://github.com/sigstore/sigstore-js/actions/runs/6014488666/attempts/1", detail.WorkflowID)
}
