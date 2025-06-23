package ratelimits

import (
	"testing"

	"github.com/letsencrypt/boulder/test"
)

func TestNewTransactionBuilder_WithBadLimitsPath(t *testing.T) {
	t.Parallel()
	_, err := NewTransactionBuilder("testdata/does-not-exist.yml", "")
	test.AssertError(t, err, "should error")

	_, err = NewTransactionBuilder("testdata/defaults.yml", "testdata/does-not-exist.yml")
	test.AssertError(t, err, "should error")
}
