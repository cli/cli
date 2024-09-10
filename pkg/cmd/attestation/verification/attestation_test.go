package verification

import (
	"os"
	"path/filepath"
	"testing"

	protobundle "github.com/sigstore/protobuf-specs/gen/pb-go/bundle/v1"
	dsse "github.com/sigstore/protobuf-specs/gen/pb-go/dsse"
	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/stretchr/testify/require"

	"github.com/cli/cli/v2/pkg/cmd/attestation/api"
)

func TestLoadBundlesFromJSONLinesFile(t *testing.T) {
	t.Run("with original file", func(t *testing.T) {
		path := "../test/data/sigstore-js-2.1.0_with_2_bundles.jsonl"
		attestations, err := loadBundlesFromJSONLinesFile(path)
		require.NoError(t, err)
		require.Len(t, attestations, 2)
	})

	t.Run("with extra lines", func(t *testing.T) {
		// Create a temporary file with extra lines
		tempDir := t.TempDir()
		tempFile := filepath.Join(tempDir, "test_with_extra_lines.jsonl")

		originalContent, err := os.ReadFile("../test/data/sigstore-js-2.1.0_with_2_bundles.jsonl")
		require.NoError(t, err)

		extraLines := []byte("\n\n")
		newContent := append(originalContent, extraLines...)

		err = os.WriteFile(tempFile, newContent, 0644)
		require.NoError(t, err)

		// Test the function with the new file
		attestations, err := loadBundlesFromJSONLinesFile(tempFile)
		require.NoError(t, err)
		require.Len(t, attestations, 2, "Should still load 2 valid attestations")
	})
}

func TestLoadBundlesFromJSONLinesFile_RejectEmptyJSONLFile(t *testing.T) {
	// Create a temporary file
	emptyJSONL, err := os.CreateTemp("", "empty.jsonl")
	require.NoError(t, err)
	err = emptyJSONL.Close()
	require.NoError(t, err)

	attestations, err := loadBundlesFromJSONLinesFile(emptyJSONL.Name())

	require.ErrorIs(t, err, ErrEmptyBundleFile)
	require.Nil(t, attestations)
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

	t.Run("with non-existent bundle file and JSON file", func(t *testing.T) {
		path := "../test/data/not-found-bundle.json"
		attestations, err := GetLocalAttestations(path)

		require.ErrorContains(t, err, "bundle could not be loaded from JSON file")
		require.Nil(t, attestations)
	})

	t.Run("with non-existent bundle file and JSON lines file", func(t *testing.T) {
		path := "../test/data/not-found-bundle.jsonl"
		attestations, err := GetLocalAttestations(path)

		require.ErrorContains(t, err, "bundles could not be loaded from JSON lines file")
		require.Nil(t, attestations)
	})

	t.Run("with missing verification material", func(t *testing.T) {
		path := "../test/data/github_provenance_demo-0.0.12-py3-none-any-bundle-missing-verification-material.jsonl"
		_, err := GetLocalAttestations(path)
		require.ErrorIs(t, err, bundle.ErrMissingVerificationMaterial)
	})

	t.Run("with missing verification certificate", func(t *testing.T) {
		path := "../test/data/github_provenance_demo-0.0.12-py3-none-any-bundle-missing-cert.jsonl"
		_, err := GetLocalAttestations(path)
		require.ErrorIs(t, err, bundle.ErrMissingBundleContent)
	})
}

func TestFilterAttestations(t *testing.T) {
	attestations := []*api.Attestation{
		{
			Bundle: &bundle.Bundle{
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
			Bundle: &bundle.Bundle{
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
			Bundle: &bundle.Bundle{
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
