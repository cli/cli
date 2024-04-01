package digest

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArtifactDigestWithAlgorithm(t *testing.T) {
	testString := "deadbeef"
	sha512TestDigest := "113a3bc783d851fc0373214b19ea7be9fa3de541ecb9fe026d52c603e8ea19c174cc0e9705f8b90d312212c0c3a6d8453ddfb3e3141409cf4bedc8ef033590b4"
	sha256TestDigest := "2baf1f40105d9501fe319a8ec463fdf4325a2a5df445adf3f572f626253678c9"

	t.Run("sha256", func(t *testing.T) {
		reader := strings.NewReader(testString)
		digest, err := CalculateDigestWithAlgorithm(reader, "sha256")
		assert.Nil(t, err)
		assert.Equal(t, sha256TestDigest, digest)
	})

	t.Run("sha512", func(t *testing.T) {
		reader := strings.NewReader(testString)
		digest, err := CalculateDigestWithAlgorithm(reader, "sha512")
		assert.Nil(t, err)
		assert.Equal(t, sha512TestDigest, digest)
	})

	t.Run("fail with sha384", func(t *testing.T) {
		reader := strings.NewReader(testString)
		_, err := CalculateDigestWithAlgorithm(reader, "sha384")
		require.Error(t, err)
		require.ErrorAs(t, err, &errUnsupportedAlgorithm)
	})
}

func TestValidDigestAlgorithms(t *testing.T) {
	t.Run("includes sha256", func(t *testing.T) {
		assert.Contains(t, ValidDigestAlgorithms(), "sha256")
	})

	t.Run("includes sha512", func(t *testing.T) {
		assert.Contains(t, ValidDigestAlgorithms(), "sha512")
	})
}
