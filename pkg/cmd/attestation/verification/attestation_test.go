package verification

import (
	"testing"

	protobundle "github.com/sigstore/protobuf-specs/gen/pb-go/bundle/v1"
	dsse "github.com/sigstore/protobuf-specs/gen/pb-go/dsse"
	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/stretchr/testify/require"

	"github.com/cli/cli/v2/pkg/cmd/attestation/api"
)

func TestLoadBundlesFromJSONLinesFile(t *testing.T) {
	path := "../test/data/sigstore-js-2.1.0_with_2_bundles.jsonl"
	attestations, err := loadBundlesFromJSONLinesFile(path)

	require.NoError(t, err)
	require.Len(t, attestations, 2)
}

func TestLoadBundleFromJSONFile(t *testing.T) {
	path := "../test/data/sigstore-js-2.1.0-bundle.json"
	attestations, err := loadBundleFromJSONFile(path)

	require.NoError(t, err)
	require.Len(t, attestations, 1)
}

func TestGetLocalAttestations(t *testing.T) {
	t.Run("with JSON file containing one bundle", func(t *testing.T) {
		path := "../test/data/sigstore-js-2.1.0-bundle.json"
		attestations, err := GetLocalAttestations(path)

		require.NoError(t, err)
		require.Len(t, attestations, 1)
	})

	t.Run("with JSON lines file containing multiple bundles", func(t *testing.T) {
		path := "../test/data/sigstore-js-2.1.0_with_2_bundles.jsonl"
		attestations, err := GetLocalAttestations(path)

		require.NoError(t, err)
		require.Len(t, attestations, 2)
	})

	t.Run("with file with unrecognized extension", func(t *testing.T) {
		path := "../test/data/sigstore-js-2.1.0-bundles.tgz"
		attestations, err := GetLocalAttestations(path)

		require.ErrorIs(t, err, ErrUnrecognisedBundleExtension)
		require.Nil(t, attestations)
	})
}

func TestFilterAttestations(t *testing.T) {
	attestations := []*api.Attestation{
		{
			Bundle: &bundle.ProtobufBundle{
				Bundle: &protobundle.Bundle{
					Content: &protobundle.Bundle_DsseEnvelope{
						DsseEnvelope: &dsse.Envelope{
							PayloadType: "application/vnd.in-toto+json",
							Payload:     []byte("{\"predicateType\": \"https://slsa.dev/provenance/v1\"}"),
						},
					},
				},
			},
		},
		{
			Bundle: &bundle.ProtobufBundle{
				Bundle: &protobundle.Bundle{
					Content: &protobundle.Bundle_DsseEnvelope{
						DsseEnvelope: &dsse.Envelope{
							PayloadType: "application/vnd.something-other-than-in-toto+json",
							Payload:     []byte("{\"predicateType\": \"https://slsa.dev/provenance/v1\"}"),
						},
					},
				},
			},
		},
		{
			Bundle: &bundle.ProtobufBundle{
				Bundle: &protobundle.Bundle{
					Content: &protobundle.Bundle_DsseEnvelope{
						DsseEnvelope: &dsse.Envelope{
							PayloadType: "application/vnd.in-toto+json",
							Payload:     []byte("{\"predicateType\": \"https://spdx.dev/Document/v2.3\"}"),
						},
					},
				},
			},
		},
	}

	filtered := FilterAttestations("https://slsa.dev/provenance/v1", attestations)

	require.Len(t, filtered, 1)

	filtered = FilterAttestations("NonExistentPredicate", attestations)
	require.Len(t, filtered, 0)
}
