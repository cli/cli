//go:build !windows

package flock_test

import (
	"errors"
	"syscall"
	"testing"

	"github.com/cli/cli/v2/internal/flock"
	"github.com/stretchr/testify/assert"
)

func TestIsUnsupportedRecognizesUnavailableUnixLocking(t *testing.T) {
	t.Parallel()

	// Given errors used by filesystems without usable flock support
	for _, err := range []error{
		syscall.ENOTSUP,
		syscall.EOPNOTSUPP,
		syscall.ENOSYS,
		syscall.ENOLCK,
	} {
		// When locking support is classified
		unsupported := flock.IsUnsupported(err)

		// Then refresh can degrade to fetching without a lock
		assert.True(t, unsupported, "expected %v to mean filesystem locking is unavailable", err)
	}

	assert.False(t, flock.IsUnsupported(errors.New("permission denied")), "unrelated lock failures must remain distinguishable")
}
