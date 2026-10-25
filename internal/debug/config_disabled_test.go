//go:build !ghdebug

package debug_test

import (
	"testing"

	"github.com/cli/cli/v2/internal/debug"
	"github.com/stretchr/testify/assert"
)

func TestConfigIgnoresEnvironment(t *testing.T) {
	for _, value := range []string{"1", "invalid"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("GH_INTERNAL_AT_EXPIRES_IN", value)
			t.Setenv("GH_INTERNAL_RT_EXPIRES_IN", value)
			t.Setenv("GH_INTERNAL_DEBUG_REFRESH_TOKEN", value)

			assert.Equal(t, debug.DebugConfig{}, debug.Config())
		})
	}
}
