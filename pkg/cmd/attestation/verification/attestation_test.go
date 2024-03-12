package verification

import (
	"testing"

	"github.com/stretchr/testify/require"
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

		require.ErrorIs(t, err, ErrLocalAttestations)
		require.Nil(t, attestations)
	})
}
