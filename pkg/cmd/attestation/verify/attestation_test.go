package verify

import (
	"testing"

	"github.com/cli/cli/v2/pkg/cmd/attestation/api"
	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact"
	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact/oci"
	"github.com/cli/cli/v2/pkg/cmd/attestation/verification"
	"github.com/stretchr/testify/require"
)

func TestGetAttestations_OCIRegistry_PredicateTypeFiltering(t *testing.T) {
	artifact, err := artifact.NewDigestedArtifact(nil, "../test/data/gh_2.60.1_windows_arm64.zip", "sha256")
	require.NoError(t, err)

	o := &Options{
		OCIClient:             oci.MockClient{},
		PredicateType:         verification.SLSAPredicateV1,
		Repo:                  "cli/cli",
		UseBundleFromRegistry: true,
	}
	attestations, msg, err := getAttestations(o, *artifact)
	require.NoError(t, err)
	require.Contains(t, msg, "Loaded 2 attestations from OCI registry")
	require.Len(t, attestations, 2)

	o.PredicateType = "custom predicate type"
	attestations, msg, err = getAttestations(o, *artifact)
	require.Error(t, err)
	require.Contains(t, msg, "no attestations found with predicate type")
	require.Nil(t, attestations)
}

func TestGetAttestations_LocalBundle_PredicateTypeFiltering(t *testing.T) {
	artifact, err := artifact.NewDigestedArtifact(nil, "../test/data/gh_2.60.1_windows_arm64.zip", "sha256")
	require.NoError(t, err)

	o := &Options{
		BundlePath:    "../test/data/sigstore-js-2.1.0-bundle.json",
		PredicateType: verification.SLSAPredicateV1,
		Repo:          "sigstore/sigstore-js",
	}
	attestations, _, err := getAttestations(o, *artifact)
	require.NoError(t, err)
	require.Len(t, attestations, 1)

	o.PredicateType = "custom predicate type"
	attestations, _, err = getAttestations(o, *artifact)
	require.Error(t, err)
	require.Nil(t, attestations)
}

func TestGetAttestations_GhAPI_NoAttestationsFound(t *testing.T) {
	artifact, err := artifact.NewDigestedArtifact(nil, "../test/data/gh_2.60.1_windows_arm64.zip", "sha256")
	require.NoError(t, err)

	o := &Options{
		APIClient:     api.NewTestClient(),
		PredicateType: verification.SLSAPredicateV1,
		Repo:          "sigstore/sigstore-js",
	}
	attestations, _, err := getAttestations(o, *artifact)
	require.NoError(t, err)
	require.Len(t, attestations, 2)

	o.PredicateType = "custom predicate type"
	attestations, _, err = getAttestations(o, *artifact)
	require.Error(t, err)
	require.Nil(t, attestations)
}
