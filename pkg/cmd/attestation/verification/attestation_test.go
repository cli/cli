package verification

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadBundlesFromJSONLinesFile(t *testing.T) {
	path := "../test/data/sigstore-js-2.1.0_with_2_bundles.jsonl"
	attestations, err := loadBundlesFromJSONLinesFile(path)

	assert.NoError(t, err)
	assert.Len(t, attestations, 2)
}

func TestLoadBundleFromJSONFile(t *testing.T) {
	path := "../test/data/sigstore-js-2.1.0-bundle.json"
	attestations, err := loadBundleFromJSONFile(path)

	assert.NoError(t, err)
	assert.Len(t, attestations, 1)
}

func TestGetLocalAttestations(t *testing.T) {
	t.Run("with JSON file containing one bundle", func(t *testing.T) {
		path := "../test/data/sigstore-js-2.1.0-bundle.json"
		attestations, err := GetLocalAttestations(path)

		assert.NoError(t, err)
		assert.Len(t, attestations, 1)
	})

	t.Run("with JSON lines file containing multiple bundles", func(t *testing.T) {
		path := "../test/data/sigstore-js-2.1.0_with_2_bundles.jsonl"
		attestations, err := GetLocalAttestations(path)

		assert.NoError(t, err)
		assert.Len(t, attestations, 2)
	})

	t.Run("with file with unrecognized extension", func(t *testing.T) {
		path := "../test/data/sigstore-js-2.1.0-bundles.tgz"
		attestations, err := GetLocalAttestations(path)

		assert.ErrorIs(t, err, ErrLocalAttestations)
		assert.Nil(t, attestations)
	})
}
