// Package debug provides build-gated configuration for internal debugging.
package debug

import (
	"time"

	o "github.com/cli/cli/v2/pkg/option"
)

// DebugConfig holds explicit internal debug overrides.
// A None field means no override; a Some field applies its value even when that value is zero or false.
type DebugConfig struct {
	// AccessTokenExpiresIn overrides the access token lifetime using GH_INTERNAL_AT_EXPIRES_IN.
	// The environment value is an integer number of seconds; None means no override.
	AccessTokenExpiresIn o.Option[time.Duration]

	// RefreshTokenExpiresIn overrides the refresh token lifetime using GH_INTERNAL_RT_EXPIRES_IN.
	// The environment value is an integer number of seconds; None means no override.
	RefreshTokenExpiresIn o.Option[time.Duration]

	// VerboseTokenRefresh overrides full refresh traffic logging using GH_INTERNAL_DEBUG_REFRESH_TOKEN.
	// The environment value must be accepted by strconv.ParseBool; None means no override.
	VerboseTokenRefresh o.Option[bool]
}
