//go:build ghdebug

package debug_test

import (
	"testing"
	"time"

	"github.com/cli/cli/v2/internal/debug"
	o "github.com/cli/cli/v2/pkg/option"
	"github.com/stretchr/testify/assert"
)

func TestConfigReadsEnvironment(t *testing.T) {
	t.Setenv("GH_INTERNAL_AT_EXPIRES_IN", "90")
	t.Setenv("GH_INTERNAL_RT_EXPIRES_IN", "120")
	t.Setenv("GH_INTERNAL_DEBUG_REFRESH_TOKEN", "true")

	assert.Equal(t, debug.DebugConfig{
		AccessTokenExpiresIn:  o.Some(90 * time.Second),
		RefreshTokenExpiresIn: o.Some(120 * time.Second),
		VerboseTokenRefresh:   o.Some(true),
	}, debug.Config())
}
