package debug

import (
	"os"
	"testing"
	"time"

	o "github.com/cli/cli/v2/pkg/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigFromEnv(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want DebugConfig
	}{
		{
			name: "unset environment",
		},
		{
			name: "configured overrides",
			env: map[string]string{
				"GH_INTERNAL_AT_EXPIRES_IN":       "90",
				"GH_INTERNAL_RT_EXPIRES_IN":       "15552000",
				"GH_INTERNAL_DEBUG_REFRESH_TOKEN": "1",
			},
			want: DebugConfig{
				AccessTokenExpiresIn:  o.Some(90 * time.Second),
				RefreshTokenExpiresIn: o.Some(180 * 24 * time.Hour),
				VerboseTokenRefresh:   o.Some(true),
			},
		},
		{
			name: "zero and negative lifetimes are explicit overrides",
			env: map[string]string{
				"GH_INTERNAL_AT_EXPIRES_IN": "0",
				"GH_INTERNAL_RT_EXPIRES_IN": "-1",
			},
			want: DebugConfig{
				AccessTokenExpiresIn:  o.Some(time.Duration(0)),
				RefreshTokenExpiresIn: o.Some(-time.Second),
			},
		},
		{
			name: "zero explicitly disables verbose logging",
			env:  map[string]string{"GH_INTERNAL_DEBUG_REFRESH_TOKEN": "0"},
			want: DebugConfig{VerboseTokenRefresh: o.Some(false)},
		},
		{
			name: "false explicitly disables verbose logging",
			env:  map[string]string{"GH_INTERNAL_DEBUG_REFRESH_TOKEN": "false"},
			want: DebugConfig{VerboseTokenRefresh: o.Some(false)},
		},
		{
			name: "true explicitly enables verbose logging",
			env:  map[string]string{"GH_INTERNAL_DEBUG_REFRESH_TOKEN": "true"},
			want: DebugConfig{VerboseTokenRefresh: o.Some(true)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearConfigEnv(t)
			for key, value := range tt.env {
				t.Setenv(key, value)
			}

			assert.Equal(t, tt.want, configFromEnv())
		})
	}
}

func TestConfigFromEnvInvalidOverride(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{name: "invalid access lifetime", key: "GH_INTERNAL_AT_EXPIRES_IN", value: "invalid"},
		{name: "invalid refresh lifetime", key: "GH_INTERNAL_RT_EXPIRES_IN", value: "invalid"},
		{name: "fractional seconds", key: "GH_INTERNAL_AT_EXPIRES_IN", value: "1.5"},
		{name: "duration overflow", key: "GH_INTERNAL_AT_EXPIRES_IN", value: "9223372037"},
		{name: "duration underflow", key: "GH_INTERNAL_AT_EXPIRES_IN", value: "-9223372037"},
		{name: "integer overflow", key: "GH_INTERNAL_RT_EXPIRES_IN", value: "9223372036854775808"},
		{name: "empty lifetime", key: "GH_INTERNAL_AT_EXPIRES_IN", value: ""},
		{name: "invalid boolean", key: "GH_INTERNAL_DEBUG_REFRESH_TOKEN", value: "enabled"},
		{name: "empty boolean", key: "GH_INTERNAL_DEBUG_REFRESH_TOKEN", value: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearConfigEnv(t)
			t.Setenv(tt.key, tt.value)

			require.Panics(t, func() {
				configFromEnv()
			})
		})
	}
}

// clearConfigEnv unsets the debug variables for the test, so cases expecting no override are not affected by the
// environment running the tests. t.Setenv registers the restore before the variable is unset.
func clearConfigEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"GH_INTERNAL_AT_EXPIRES_IN",
		"GH_INTERNAL_RT_EXPIRES_IN",
		"GH_INTERNAL_DEBUG_REFRESH_TOKEN",
	} {
		t.Setenv(key, "")
		require.NoError(t, os.Unsetenv(key))
	}
}
