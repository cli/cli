package download

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAreFlagsValid(t *testing.T) {
	t.Run("missing Owner", func(t *testing.T) {
		opts := Options{
			DigestAlgorithm: "sha512",
		}

		err := opts.AreFlagsValid()
		assert.Error(t, err)
		assert.ErrorContains(t, err, "owner must be provided")
	})

	t.Run("missing DigestAlgorithm", func(t *testing.T) {
		opts := Options{
			Owner: "github",
		}

		err := opts.AreFlagsValid()
		assert.Error(t, err)
		assert.ErrorContains(t, err, "digest-alg cannot be empty")
	})

	t.Run("Limit is too low", func(t *testing.T) {
		opts := Options{
			DigestAlgorithm: "sha512",
			Limit:           0,
			Owner:           "github",
		}

		err := opts.AreFlagsValid()
		assert.Error(t, err)
		assert.ErrorContains(t, err, "limit 0 not allowed, must be between 1 and 1000")
	})

	t.Run("Limit is too high", func(t *testing.T) {
		opts := Options{
			DigestAlgorithm: "sha512",
			Limit:           1001,
			Owner:           "github",
		}

		err := opts.AreFlagsValid()
		assert.Error(t, err)
		assert.ErrorContains(t, err, "limit 1001 not allowed, must be between 1 and 1000")
	})
}
